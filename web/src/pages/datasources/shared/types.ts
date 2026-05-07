// Flat shape antd's <Form> binds against for the data-source drawer.
// Kept in its own module so per-kind form components can `import type`
// only this interface without dragging in the orchestrator.
//
// The `prom_*` / `k8s_*` / `os_*` / `kafka_*` prefixes keep secrets and
// per-kind config explicitly separate from the cross-kind base fields,
// so `formToPayload` (in serialize.ts) can route each value to the
// correct backend slot without ambiguity.

import type { DataSourceKind, K8sEventKind } from '../../../types'

export interface DataSourceFormShape {
  name: string
  kind: DataSourceKind
  description?: string
  endpoint?: string
  is_enabled: boolean
  is_default: boolean
  // Enables manual AI analysis for incidents from this source (kafka / OS / elastic).
  ai_enabled: boolean
  // Create incident → auto-enqueue AI task (only if ai_enabled).
  ai_auto_trigger: boolean

  // prometheus
  prom_auth_type?: 'none' | 'basic' | 'bearer'
  prom_username?: string
  prom_token?: string
  prom_password?: string
  prom_tls_skip?: boolean

  // k8s
  k8s_in_cluster?: boolean
  k8s_token?: string
  k8s_tls_skip?: boolean
  k8s_watched_namespaces?: string
  k8s_ignored_namespaces?: string
  k8s_ignored_pods_re?: string
  k8s_events?: K8sEventKind[]
  k8s_mute_seconds?: number
  k8s_ignore_restart_count_above?: number
  k8s_pending_threshold_seconds?: number

  // opensearch / elastic (LOG_STORE_KINDS share this slot)
  os_username?: string
  os_password?: string
  os_index?: string
  os_query?: string
  os_watermark_field?: string
  os_poll_interval_seconds?: number
  os_lookback_seconds?: number
  os_tls_skip?: boolean
  os_consumer_concurrency?: number

  // kafka
  kafka_topic?: string
  kafka_group_id?: string
  kafka_sasl_mechanism?: '' | 'PLAIN' | 'SCRAM-SHA-256' | 'SCRAM-SHA-512'
  kafka_sasl_user?: string
  kafka_sasl_password?: string
  kafka_tls_enabled?: boolean
  kafka_tls_skip?: boolean
  kafka_max_per_second?: number
  kafka_consumer_concurrency?: number
  kafka_filter?: string
  kafka_map_alertname?: string
  kafka_map_severity?: string
  kafka_map_fingerprint?: string
  kafka_map_starts_at?: string
  kafka_map_ends_at?: string
  kafka_map_summary?: string
  kafka_map_description?: string
  kafka_map_status_path?: string
  kafka_map_resolved_when?: string
  kafka_labels?: { key?: string; path?: string }[]
  kafka_annotations?: { key?: string; path?: string }[]
}
