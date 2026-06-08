// Shared constants for the data-sources page.  Extracted from the
// monolithic DataSources.tsx so per-kind form components can pull
// just what they need without dragging in the whole orchestrator.

import type { DataSourceKind, K8sEventKind } from '../../../types'

// Masked sentinel returned by the server in `secret_keys`-populated edits.
// The form mirrors it back as the `password` value so "leave blank to
// keep" works without exposing the ciphertext to the browser.
export const SECRET_MASK = '******'

// Top-level filter tabs on the page header.  "all" is the default landing
// view; the per-kind tabs filter the list AND pre-fill the kind in the
// create drawer.  Mirrors the backend `allowedDataSourceKinds`
// (router/data_sources.go) — keep both lists in sync.
export const KIND_TABS: { value: 'all' | DataSourceKind; label: string }[] = [
  { value: 'all',        label: '全部' },
  { value: 'prometheus', label: 'Prometheus' },
  { value: 'k8s',        label: 'Kubernetes' },
  { value: 'opensearch', label: 'OpenSearch' },
  { value: 'elastic',    label: 'Elasticsearch' },
  { value: 'kafka',      label: 'Kafka' },
]

export const KIND_LABEL: Record<DataSourceKind, string> = {
  prometheus: 'Prometheus',
  k8s:        'Kubernetes',
  opensearch: 'OpenSearch',
  elastic:    'Elasticsearch',
  kafka:      'Kafka',
}

// Closed enum mirrored on the backend (router/data_sources.go::allowedK8sEvents).
export const K8S_EVENT_OPTIONS: { value: K8sEventKind; label: string; hint: string }[] = [
  { value: 'pod_restart',       label: 'Pod 异常重启',     hint: 'RestartCount 增长 + 上一次容器非零退出' },
  { value: 'pod_pending',       label: 'Pod 长时间 Pending', hint: '超过阈值仍未调度 / 拉镜像失败 / Init 容器卡住' },
  { value: 'hpa_scale',         label: 'HPA 扩容 / 缩容',   hint: 'Deployment / StatefulSet 副本数变化' },
  { value: 'node_not_ready',    label: 'Node NotReady',    hint: 'Node Ready=False / SchedulingDisabled' },
  { value: 'failed_scheduling', label: 'FailedScheduling', hint: '原生 Event：资源不足 / NoMatchingPVC 等' },
]

// Kinds that may use AI (manual and optional auto-enqueue) — kafka / opensearch /
// elastic.  Mirrors `internal/ai/eligibility.go::ShouldRun` and the backend
// validator for ai_enabled.  Used by the table column and drawer toggles.
export const AI_SUPPORTED_KINDS: ReadonlySet<DataSourceKind> = new Set<DataSourceKind>([
  'kafka',
  'opensearch',
  'elastic',
  'prometheus',
  'k8s',
])

export function isAIEligibleKind(kind: DataSourceKind | undefined): boolean {
  if (!kind) return false
  return AI_SUPPORTED_KINDS.has(kind)
}

// Kinds that share the OpenSearch HTTP query DSL + Basic-Auth credential
// shape.  The backend treats them identically (`router/data_sources.go`
// every OpenSearch case is extended with a parallel Elastic case); the
// frontend uses this set to drive the shared LogStoreForm component.
export const LOG_STORE_KINDS: ReadonlySet<DataSourceKind> = new Set<DataSourceKind>([
  'opensearch',
  'elastic',
])

export function isLogStoreKind(kind: DataSourceKind | undefined): boolean {
  if (!kind) return false
  return LOG_STORE_KINDS.has(kind)
}
