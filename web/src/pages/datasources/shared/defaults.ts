// Per-kind initial values used by the orchestrator when:
//   1. opening the create drawer (pre-populates the empty form), or
//   2. switching kinds inside the drawer (resets stale per-kind state).
//
// Keep field defaults aligned with the matching backend validators in
// internal/router/data_sources.go so the form's "save as-is" never
// trips a 400.

import type { DataSourceKind } from '../../../types'
import type { DataSourceFormShape } from './types'

export function kindDefaults(kind: DataSourceKind): Partial<DataSourceFormShape> {
  switch (kind) {
    case 'prometheus':
      return { prom_auth_type: 'none' }
    case 'k8s':
      return {
        k8s_in_cluster: false,
        k8s_events: ['pod_restart'],
        k8s_mute_seconds: 1800,
        k8s_ignore_restart_count_above: 20,
        k8s_pending_threshold_seconds: 300,
      }
    case 'opensearch':
    case 'elastic':
      // Elasticsearch and OpenSearch share the HTTP query DSL +
      // Basic-Auth credential shape, so their form defaults are
      // identical.  See LogStoreForm.tsx for the rendered fields.
      return {
        os_watermark_field: '@timestamp',
        os_poll_interval_seconds: 30,
        os_lookback_seconds: 300,
        os_consumer_concurrency: 1,
      }
    case 'kafka':
      // Pre-filled draft tailored for an nginx / Higress-style access
      // log topic (the most common Kafka source we see).  All values
      // are valid syntax against the v3 dual-mode mapping (gjson by
      // default, `expr:` opt-in) and produce a working dry-run against
      // the bundled NGINX_GATEWAY_SAMPLE — operators can save as-is or
      // tweak in place.
      return {
        kafka_sasl_mechanism: '',
        kafka_max_per_second: 200,
        kafka_consumer_concurrency: 1,
        kafka_filter: 'response_body != "-"',
        kafka_map_alertname:   'route_name',
        kafka_map_severity:    'expr: response_code >= "500" ? "P2" : "P3"',
        kafka_map_summary:     'expr: method + " " + path + " -> " + response_code',
        kafka_map_description: 'response_body',
        kafka_map_starts_at:   'start_time',
        kafka_map_ends_at:     '',
        kafka_map_fingerprint: 'expr: route_name + "|" + normalize_path(strip_query(path))',
        kafka_map_status_path: '',
        kafka_map_resolved_when: '',
        kafka_labels: [
          { key: 'route_name',     path: 'route_name' },
          { key: 'path_template',  path: 'expr: normalize_path(strip_query(path))' },
          { key: 'method',         path: 'method' },
          { key: 'code',           path: 'response_code' },
          { key: 'true_client_ip', path: 'expr: coalesce(true_client_ip, downstream_remote_address)' },
        ],
        kafka_annotations: [
          { key: 'request_id',            path: 'request_id' },
          { key: 'upstream',              path: 'upstream_host' },
          { key: 'response_body',         path: 'response_body' },
          { key: 'response_code_details', path: 'response_code_details' },
        ],
      }
  }
}
