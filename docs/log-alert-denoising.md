# 日志三层降噪：Kafka / OpenSearch / Elastic 在 AlertMesh 中的落地

本文说明如何把业界常见的 **「日志 → 告警事件 → 事故/通知」** 与 AlertMesh 现有模块对齐，并标明 **进程内已实现** 与 **建议在外部或后续迭代完成** 的部分。

## 三层与 AlertMesh 模块对应

| 层级 | 目标 | 在 AlertMesh 中的落点 |
|------|------|----------------------|
| 第 1 层 | 把海量日志收成少量、可稳定分组的「告警事件」 | **Kafka**：`data_sources.config` 的 `filter` + `mapping`（含 `expr:`）；**OpenSearch/Elastic**：推荐 **Monitor/规则聚合 + Webhook** 入站（见下文）；或上游 Flink/Logstash 写 Kafka |
| 第 2 层 | 去重、静默、路由、短时合并、抑制 | [`internal/engine/pipeline.go`](../internal/engine/pipeline.go)：dedup → silence → route → aggregate → inhibit |
| 第 3 层 | 事故、重复通知、升级 | [`internal/incident/`](../internal/incident/) + [`internal/notification/dispatcher.go`](../internal/notification/dispatcher.go) |

引擎聚合语义见 [`internal/engine/aggregation.go`](../internal/engine/aggregation.go)：**按 label 子集算 `group_key`**，**`group_wait` 从该 key 首次出现起计时**（同 key 内后续条不重置定时器）。这与 Alertmanager 的 `group_interval` 滑动语义不同，调参时需知悉。

**当前引擎缺口（可选后续产品化）**

- **固定日历时间窗计数**（如「5 分钟内 ERROR ≥ 50」）：未作为一等公民实现；宜放在第 1 层（OS/ES Alerting、Flink）或 OS 查询聚合。
- **Flapping 检测**：仅有 **fingerprint + TTL** 去重（默认 5 分钟，见 [`internal/engine/dedup.go`](../internal/engine/dedup.go)），无独立 flapping 状态机。
- **按数据源配置 dedup TTL / 滑动 group_wait**：未实现；若需要可单独立项。

---

## 一、Kafka 数据源（进程内已接入 Pipeline）

### 1.1 第 1 层：映射成「告警事件」（RawAlert）

在 **数据源 → Kafka → 字段映射/过滤** 中配置（详见 [data-sources.md §5](data-sources.md#5-kafka-数据源filter-表达式--字段映射)）。

**推荐标签（供第 2 层 `group_by` 使用）**

| Label 含义 | 建议来源 |
|-------------|----------|
| `service` / `app` | 服务名 |
| `env` | `prod` / `staging` |
| `error_code` / `exception_class` | 稳定错误类型 |
| `api` 或归一化 `path` | `expr: normalize_path(strip_query(path))`，避免 UUID 段撑爆基数 |
| `host` / `cluster` | 节点或集群 |

**`fingerprint`（去重键，dedup 层）**

- 推荐：`expr: route_name + "|" + normalize_path(strip_query(path))` 或与 `exception_class` 组合。
- **避免**：用 `traceId` 作主键（基数过高）；用**整条** `message` 作主键（动态参数导致无法合并同类错误）。

**`filter`**

- 排除型条件请用文档中的 `neq()` / `has()` 等，避免 `level != "DEBUG"` 在缺字段时误放行。

### 1.2 第 2 层：路由与聚合策略

- **聚合策略**（`aggregation_policies`）： matchers 命中后使用其 `group_by` 与 `group_wait`（秒）。
- **告警路由**（`alert_routes`）：未命中策略时使用路由的 `group_by`。
- 默认仅 `alertname`；日志类应显式配置 **与 Kafka labels 对齐的 `group_by`**。

配置入口：告警中心 Web UI「聚合策略」「告警路由」；加载逻辑见 [`internal/engine/pipeline.go`](../internal/engine/pipeline.go) `loadAggregations` / `loadRoutes`。

### 1.3 第 3 层

同 `group_key` 的 Incident 在 repeat 窗口内会 **append**；通知阶梯与严重级升级见通知策略相关文档。

### 1.4 若需「时间窗条数阈值」

在 **Kafka 上游**（Flink / 自定义作业 / OS 查询结果写 topic）先聚合，再让 AlertMesh 消费 **已稀疏** 的事件；或在 **OpenSearch Alerting** 做 bucket 阈值后通过 Webhook 送达（见第三节）。

---

## 二、OpenSearch / Elastic：进程内拉取（路线 B）现状

- `data_sources` 已支持 `opensearch` / `elastic` 的 **注册、凭证、DSL 配置** 与 UI。
- **主进程未启动** OpenSearch 轮询 ingest（[`cmd/alertmesh/main.go`](../cmd/alertmesh/main.go) 仅 `StartKafka`）；与 AI 工具使用的环境变量 `OPENSEARCH_URL` 是 **两条线**。

### 路线 B — 实现里程碑清单（未在代码库交付）

以下项适合拆独立 PR / 版本，而非与 Webhook 路线混在同一变更中：

1. **HTTP 客户端**：复用 `data_sources` 的 endpoint + Basic-Auth（与 [`internal/router/data_sources.go`](../internal/router/data_sources.go) `testOpenSearch` 一致的 TLS/凭据解密）。
2. **水位与分页**：按 `watermark_field`（默认 `@timestamp`）+ `search_after` 或 scroll/PIT，避免全表扫描。
3. **查询形态**：优先 **聚合查询**（bucket + threshold）在 ES/OS 内完成，只在超阈值时构造 1 条 `RawAlert`；原始日志逐条 ingest 仅适合低量 PoC。
4. **映射**：Kafka `expr:` 与 gjson 的混合能力或独立 JSONPath 表，将命中结果映射为 `alertname` / `severity` / `labels` / `fingerprint`。
5. **生命周期**：[`cmd/alertmesh/main.go`](../cmd/alertmesh/main.go) 启动 goroutine；订阅 `data_source_event`（与 [`internal/ingestion/kafka_manager.go`](../internal/ingestion/kafka_manager.go) 相同 NOTIFY 通道）实现热加载/停启。
6. **背压**：`consumer_concurrency`、`max_per_second` 等价限流，失败写入 `data_sources.last_error`。

**路线 B（未来里程碑）** 若要在进程内闭环，按上表逐项落地；大索引下应优先 **查询侧聚合**（`date_histogram` / `composite`）再产出少量命中，避免逐行灌引擎。

---

## 三、OpenSearch / Elastic：Webhook 入站（路线 A，推荐）

将 **OpenSearch Dashboards Alerting** 或 **Elastic/Kibana 规则** 的 **bucket / threshold** 与 **throttling** 放在日志平台侧完成，AlertMesh 只做 **第 2/3 层**。

### 3.1 入站端点

- `POST /api/v1/alerts/webhook/{source}`，**RFC 9421** 签名（与「Webhook 可信源」管理页一致）。
- 在 **告警中心 → Webhook 可信源** 创建 `{source}`，配置 **`mapping`**（JSON），将上游 JSON 的 **gjson 路径** 映射到 `alertname` / `severity` 等（支持点号路径，如 `monitor.name`）。

### 3.2 映射 JSON 形状（与 `ingestion.WebhookMapping` 一致）

```json
{
  "alertname_path": "monitor_name",
  "severity_path": "severity",
  "description_path": "error",
  "starts_at_path": "period_start",
  "fingerprint_path": "monitor_id",
  "summary_path": "trigger_name",
  "service_path": "monitor_name",
  "label_paths": {
    "monitor_id": "monitor_id",
    "trigger_name": "trigger_name"
  }
}
```

**说明**

- 路径语法为 [tidwall/gjson](https://github.com/tidwall/gjson)（与 Kafka mapping 生态一致），支持嵌套字段与数组下标（如 `results.0.key`）。
- `starts_at_path`：支持 **RFC3339 字符串** 或 **Unix 毫秒/秒** 数字。
- `label_paths`：额外写入 `RawAlert.labels`（用于路由 matcher 与 `group_by`）。
- 上游实际字段名因 **Monitor 类型与版本** 而异，请以你集群真实 Webhook body 为准，用 `mapping` 对齐。

### 3.3 与第 2 层衔接

Webhook 产出的 `RawAlert` 与 Kafka 一样进入 **同一套** Pipeline；请在 **告警路由 / 聚合策略** 里为 `source=<你的webhook源名>` 或 `service`/`env` 等 label 配置 `group_by` 与 `group_wait`。

---

## 四、文档与代码索引

| 主题 | 位置 |
|------|------|
| Kafka filter / mapping DSL | [data-sources.md §5](data-sources.md#5-kafka-数据源filter-表达式--字段映射) |
| 数据流与 Incident | [data-flow.md](data-flow.md) |
| Webhook 签名与安全 | [safety.md](safety.md)（若存在）/ 代码 `internal/router/middleware/webhook_signature.go` |

---

## 五、实施顺序建议

1. Kafka：按上表调 **labels + fingerprint + filter**，并配置 **聚合策略 / 路由 `group_by`**。  
2. OpenSearch/Elastic：先 **路线 A**（Monitor → Webhook → 配置 `mapping`）。  
3. 评估是否投入 **路线 B（进程内 poller）** 与引擎增强（dedup TTL、滑动窗口、flapping）。

---

## 六、Kafka 服务日志告警 + 手动 AI（OpenSearch / Prometheus）

本节对应「Kafka 入站服务日志告警 → Incident → 人工点 **触发 AI 分析** → `metrics_query` / `logs_search` / `system_info` 拉 Prom + OS」的落地清单。**AI 工具只读进程环境变量** `ALERTMESH_PROMETHEUS_URL` / `ALERTMESH_OPENSEARCH_URL`（见 [`internal/config/config.go`](../internal/config/config.go)），与 `data_sources` 里是否注册了 OpenSearch **数据源行**无关；后者用于 UI 凭证与（未来）路线 B 轮询。

### 6.1 Kafka：`filter` / `mapping.labels` / `fingerprint` + 聚合 `group_by`

**目标**：标签带上 **服务、命名空间、Pod、节点**，指纹稳定（避免 traceId / 整条 message），第 2 层 `group_by` 与这些 label **对齐**，避免一屏一 Incident。

**`data_sources.config` 片段示例**（字段路径请按你 topic 的 JSON 替换；`labels` 键名与下文聚合策略一致即可）：

```json
{
  "topic": "service-log-alerts",
  "group_id": "alertmesh-ingest",
  "filter": "oneof(level, 'ERROR', 'CRITICAL') && oneof(env, 'prod')",
  "mapping": {
    "alertname": "alert.rule",
    "severity": "alert.severity",
    "fingerprint": "expr: coalesce(service, app) + \"|\" + normalize_path(strip_query(http_path)) + \"|\" + coalesce(exception_class, error_code, \"unknown\")",
    "starts_at": "timestamp",
    "summary": "message",
    "description": "stack_trace",
    "labels": {
      "service": "service",
      "app": "app",
      "env": "env",
      "namespace": "k8s.namespace",
      "pod": "k8s.pod",
      "node": "k8s.node",
      "host": "host"
    }
  }
}
```

**聚合策略（Web UI 或 API）**：`matchers` 命中该 Kafka 源的告警后，`group_by` 建议包含与上表一致的键，例如：

```json
["service", "namespace", "pod", "alertname"]
```

或按噪声情况收紧为 `["service", "namespace", "exception_class"]`。**告警路由**在未命中策略时的 `group_by` 也应与日志类 label 对齐（见 [`internal/engine/aggregation.go`](../internal/engine/aggregation.go) 的 group_key 语义）。

### 6.2 进程环境与数据源开关（`ai_enabled` / `ai_auto_trigger`）

| 项 | 说明 |
|----|------|
| `ALERTMESH_PROMETHEUS_URL` | Prometheus 根 URL，供 **`metrics_query`** |
| `ALERTMESH_OPENSEARCH_URL` | OpenSearch 根 URL，供 **`logs_search`** / **`system_info`** |
| Kafka 数据源 | UI：`ai_enabled` **开启**；`ai_auto_trigger` **关闭**（默认），仅人工点「触发 AI 分析」时跑 LLM（见 [`internal/incident/service.go`](../internal/incident/service.go)） |

### 6.3 OpenSearch：应用日志 + syslog（+ 可选 K8s Event 进索引）

AI 工具在单集群上查询；常用 **索引模式** 可由运维在 OS 侧统一别名，例如：

| 用途 | 建议 index pattern / alias | 与工具的对应关系 |
|------|---------------------------|------------------|
| 应用 stdout / 业务日志 | `app-logs-*` 或 `logs-app-*` | `logs_search` 的 query 命中 `message` / `service` / `level` |
| syslog / 主机日志 | `syslog-*` | **`system_info`** 同类字段；`_source` 仍含 `@timestamp`、`message`、`host` 等（见 [`internal/ai/tools/logs.go`](../internal/ai/tools/logs.go)） |
| K8s Event（无独立 K8s API 工具时） | `k8s-events-*` | 将 Event 以 JSON 行写入 OS，仍用 **`logs_search`** 按时间检索 |

**字段对齐建议**（便于 PromQL / 日志关联）：`@timestamp`（ISO8601）、`service`、`host`、`namespace`、`pod`（或与 `kubernetes.pod_name` 二选一并在 ingest 时归一成工具 `_source` 里的 `pod`）。工具默认 `_source` 包含 `namespace` / `pod` / `container`；若你索引用嵌套 `kubernetes.*`，请在 ingest pipeline 中复制到顶层或扩展工具映射（后续迭代）。

### 6.4 分析 prompt 中的时间锚点

根因分析系统 prompt 会注入 **Incident `opened_at`** 与 **已挂载告警的 `starts_at` 最小/最大范围**（UTC），并提示 `logs_search` 的时间窗为 **[now − time_range, now]**，引导模型选择足够长的 `time_range` 与重叠的 Prom 区间。实现见 [`internal/ai/agent.go`](../internal/ai/agent.go) `analysisTimeAnchorBlock`。
