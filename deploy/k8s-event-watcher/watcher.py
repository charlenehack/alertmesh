#!/usr/bin/env python3
"""
K8s Event Watcher — 将集群中所有 Event 实时推送到 AlertMesh。

部署方式：
  1. 集群内：ServiceAccount + Deployment（推荐，in-cluster 认证）
  2. 集群外：设置 KUBECONFIG 环境变量

推送到 AlertMesh 的 /api/v1/alerts/alertmanager 端点，
复用 Alertmanager 适配器，零配置即用。
"""

import json
import os
import sys
import time
import hashlib
import logging
import signal
from datetime import datetime, timezone

import requests
from kubernetes import config as k8s_config

# ─── 配置 ──────────────────────────────────────────────────────────────────────

ALERTMESH_URL = os.environ.get("ALERTMESH_URL", "http://10.11.12.146:8081")
# Alertmanager webhook 端点，无需鉴权
WEBHOOK_PATH = "/api/v1/alerts/alertmanager"

# 仅推送 Warning 类型事件（设为 false 则推送所有类型包括 Normal）
WARNING_ONLY = os.environ.get("WARNING_ONLY", "true").lower() == "true"

# Event 的 resourceVersion 缓存文件（重启后不重复推送）
CACHE_FILE = os.environ.get("CACHE_FILE", "/tmp/k8s-event-watcher-rv")

# 推送失败重试次数
MAX_RETRIES = int(os.environ.get("MAX_RETRIES", "3"))

# 推送超时（秒）
PUSH_TIMEOUT = int(os.environ.get("PUSH_TIMEOUT", "10"))

# 批量聚合窗口（秒）—— 同一秒内的事件打包推送
BATCH_WINDOW = float(os.environ.get("BATCH_WINDOW", "1.0"))

# 集群名称（写入 label，多集群时区分）
CLUSTER_NAME = os.environ.get("CLUSTER_NAME", "")

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
log = logging.getLogger("k8s-event-watcher")

# ─── K8s Event → AlertMesh Alert 映射 ──────────────────────────────────────────

def _raw_event_to_alert(obj: dict) -> dict | None:
    """将原始 K8s Event dict 映射为 Alertmanager 格式的 alert dict。"""
    involved = obj.get("involvedObject") or {}
    etype = obj.get("type") or "Normal"

    if WARNING_ONLY and etype != "Warning":
        return None

    reason = obj.get("reason") or "K8sEvent"
    namespace = involved.get("namespace") or "default"
    kind = involved.get("kind") or "Unknown"
    name = involved.get("name") or "unknown"

    if etype == "Warning":
        severity = "P1" if _is_critical_reason(reason) else "P2"
    else:
        severity = "P3"

    labels = {
        "alertname": reason,
        "severity": severity,
        "source": "k8s",
        "namespace": namespace,
        "kind": kind,
        "name": name,
        "event_type": etype,
    }
    if CLUSTER_NAME:
        labels["cluster"] = CLUSTER_NAME
    src = obj.get("source") or {}
    if src.get("component"):
        labels["source_component"] = src["component"]
    if src.get("host"):
        labels["source_host"] = src["host"]

    message = obj.get("message") or ""
    annotations = {
        "summary": f"[{etype}] {kind}/{namespace}/{name}: {message}",
        "description": message,
    }
    if obj.get("lastTimestamp"):
        annotations["last_timestamp"] = obj["lastTimestamp"]
    if obj.get("count"):
        annotations["count"] = str(obj["count"])

    fp_input = f"{namespace}/{kind}/{name}/{reason}"
    fingerprint = hashlib.sha256(fp_input.encode()).hexdigest()[:16]

    starts_at = obj.get("lastTimestamp") or obj.get("eventTime") or \
        (obj.get("metadata") or {}).get("creationTimestamp")
    if not starts_at:
        starts_at = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    return {
        "status": "firing",
        "labels": labels,
        "annotations": annotations,
        "startsAt": starts_at,
        "endsAt": "0001-01-01T00:00:00Z",
        "fingerprint": fingerprint,
    }



def _is_critical_reason(reason: str | None) -> bool:
    """判断是否为高严重级别 Reason。"""
    if not reason:
        return False
    critical_keywords = [
        "OOMKilling", "OOMKilled", "Killed", "FailedScheduling",
        "NodeNotReady", "Unhealthy", "BackOff", "CrashLoopBackOff",
        "FailedMount", "FailedAttachVolume", "FailedKillPod",
    ]
    return any(kw in (reason or "") for kw in critical_keywords)


# ─── 推送到 AlertMesh ──────────────────────────────────────────────────────────

def push_alerts(alerts: list[dict]) -> bool:
    """批量推送 alerts 到 AlertMesh Alertmanager 端点。"""
    payload = {
        "status": "firing",
        "alerts": alerts,
    }
    body = json.dumps(payload, default=str).encode()
    url = f"{ALERTMESH_URL.rstrip('/')}{WEBHOOK_PATH}"

    for attempt in range(1, MAX_RETRIES + 1):
        try:
            resp = requests.post(
                url,
                data=body,
                headers={"Content-Type": "application/json"},
                timeout=PUSH_TIMEOUT,
            )
            if resp.status_code in (200, 202):
                log.info("推送 %d 条告警 → %s [OK]", len(alerts), url)
                return True
            log.warning("推送失败 [HTTP %d]: %s", resp.status_code, resp.text[:200])
        except requests.RequestException as e:
            log.warning("推送异常 (尝试 %d/%d): %s", attempt, MAX_RETRIES, e)
        if attempt < MAX_RETRIES:
            time.sleep(2 ** attempt)
    return False


# ─── Watch 主循环 ──────────────────────────────────────────────────────────────

def load_resource_version() -> str:
    """从缓存文件加载上次Watch的resourceVersion，避免重启后重复推送。"""
    try:
        with open(CACHE_FILE) as f:
            return f.read().strip()
    except FileNotFoundError:
        return ""


def save_resource_version(rv: str):
    """持久化当前 resourceVersion。"""
    try:
        with open(CACHE_FILE, "w") as f:
            f.write(rv)
    except OSError as e:
        log.warning("写入缓存失败: %s", e)


def _load_k8s_credentials() -> tuple[str, str, bool]:
    """
    解析认证信息，返回 (api_server, token, verify_ssl)。
    优先 in-cluster ServiceAccount，其次 kubeconfig。
    """
    # in-cluster：读 /var/run/secrets/kubernetes.io/serviceaccount/
    sa_token_path = "/var/run/secrets/kubernetes.io/serviceaccount/token"
    sa_host = os.environ.get("KUBERNETES_SERVICE_HOST", "")
    sa_port = os.environ.get("KUBERNETES_SERVICE_PORT", "443")
    if sa_host and os.path.exists(sa_token_path):
        token = open(sa_token_path).read().strip()
        api_server = f"https://{sa_host}:{sa_port}"
        log.info("使用 in-cluster 认证: %s", api_server)
        return api_server, token, True

    # 集群外：解析 kubeconfig
    kubeconfig_path = os.path.expanduser(
        os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    import yaml
    with open(kubeconfig_path) as f:
        kc = yaml.safe_load(f)
    current_ctx = kc.get("current-context", "")
    ctx_obj = next((c["context"] for c in kc.get("contexts", []) if c["name"] == current_ctx), {})
    cluster_name = ctx_obj.get("cluster", "")
    user_name = ctx_obj.get("user", "")
    cluster_obj = next((c["cluster"] for c in kc.get("clusters", []) if c["name"] == cluster_name), {})
    user_obj = next((u["user"] for u in kc.get("users", []) if u["name"] == user_name), {})
    api_server = cluster_obj.get("server", "")
    token = user_obj.get("token", "")
    verify_ssl = not cluster_obj.get("insecure-skip-tls-verify", False)
    log.info("使用 kubeconfig 认证: %s (user=%s, token_len=%d)", api_server, user_name, len(token))
    return api_server, token, verify_ssl


def watch_events():
    """主 Watch 循环，自动重连。"""
    api_server, token, verify_ssl = _load_k8s_credentials()
    headers = {"Authorization": "Bearer " + token}

    # 获取初始 resourceVersion
    last_rv = load_resource_version()
    if last_rv:
        log.info("从 resourceVersion=%s 继续", last_rv[:20] + "..." if len(last_rv) > 20 else last_rv)

    shutdown = False

    def handle_signal(signum, frame):
        nonlocal shutdown
        log.info("收到信号 %d，优雅退出...", signum)
        shutdown = True

    signal.signal(signal.SIGTERM, handle_signal)
    signal.signal(signal.SIGINT, handle_signal)

    batch: list[dict] = []
    last_flush = time.monotonic()

    while not shutdown:
        try:
            url = f"{api_server}/api/v1/events?watch=true&timeoutSeconds=300"
            if last_rv:
                url += f"&resourceVersion={last_rv}"
            log.info("开始 Watch Events%s...", f" (rv={last_rv[:16]}...)" if last_rv else "")

            resp = requests.get(
                url,
                headers=headers,
                verify=verify_ssl,
                stream=True,
                timeout=(10, 310),
            )
            if resp.status_code != 200:
                body = resp.text[:300]
                log.error("Watch 请求失败 HTTP %d: %s", resp.status_code, body)
                if resp.status_code == 410:
                    log.warning("resourceVersion 已过期 (410 Gone)，重新全量同步")
                    last_rv = ""
                    save_resource_version("")
                time.sleep(5)
                continue

            for line in resp.iter_lines():
                if shutdown:
                    break
                if not line:
                    continue
                try:
                    evt = json.loads(line)
                except json.JSONDecodeError:
                    continue

                evt_type = evt.get("type", "")
                obj_raw = evt.get("object", {})

                # 更新 resourceVersion
                rv = (obj_raw.get("metadata") or {}).get("resourceVersion", "")
                if rv:
                    last_rv = rv

                if evt_type == "DELETED":
                    continue
                if evt_type == "ERROR":
                    reason = (obj_raw.get("reason") or "").lower()
                    if "expired" in reason or "gone" in reason:
                        log.warning("resourceVersion 已过期，重新全量同步")
                        last_rv = ""
                        save_resource_version("")
                    break

                alert = _raw_event_to_alert(obj_raw)
                if alert:
                    batch.append(alert)

                now = time.monotonic()
                if batch and (now - last_flush >= BATCH_WINDOW or len(batch) >= 50):
                    push_alerts(batch)
                    save_resource_version(last_rv)
                    batch.clear()
                    last_flush = now

            # stream 结束，推送残余
            if batch:
                push_alerts(batch)
                save_resource_version(last_rv)
                batch.clear()
                last_flush = time.monotonic()

        except requests.RequestException as e:
            log.error("Watch 请求异常: %s", e)
        except Exception as e:
            log.error("Watch 异常: %s", e)

        if not shutdown:
            log.info("5 秒后重连...")
            time.sleep(5)

    # 退出前推送残余
    if batch:
        push_alerts(batch)
        save_resource_version(last_rv)

    log.info("K8s Event Watcher 已停止")


if __name__ == "__main__":
    log.info("K8s Event Watcher 启动")
    log.info("  ALERTMESH_URL  = %s", ALERTMESH_URL)
    log.info("  WARNING_ONLY   = %s", WARNING_ONLY)
    log.info("  CLUSTER_NAME   = %s", CLUSTER_NAME or "(未设置)")
    watch_events()
