// Kafka mapping helpers shared by the form, the test-message modal and
// the orchestrator's save-mutation error handler.  Centralising here so
// that adding a new mapping field only requires touching one map.

import type { FormInstance } from 'antd'
import type { DataSourceFormShape } from './types'

// kafkaMappingFromForm collapses the flat form shape into the JSON
// object the backend's KafkaMapping expects.  Empty paths drop out so
// we don't pollute the stored config with no-op keys.
export function kafkaMappingFromForm(v: DataSourceFormShape): Record<string, unknown> {
  const out: Record<string, unknown> = {
    alertname:     v.kafka_map_alertname || '',
    severity:      v.kafka_map_severity || '',
    fingerprint:   v.kafka_map_fingerprint || '',
    starts_at:     v.kafka_map_starts_at || '',
    ends_at:       v.kafka_map_ends_at || '',
    summary:       v.kafka_map_summary || '',
    description:   v.kafka_map_description || '',
    status_path:   v.kafka_map_status_path || '',
    resolved_when: v.kafka_map_resolved_when || '',
  }
  const labels: Record<string, string> = {}
  for (const r of v.kafka_labels || []) {
    if (r?.key && r?.path) labels[r.key] = r.path
  }
  out.labels = labels
  const annotations: Record<string, string> = {}
  for (const r of v.kafka_annotations || []) {
    if (r?.key && r?.path) annotations[r.key] = r.path
  }
  out.annotations = annotations
  return out
}

// Backend mapping key → form field name.  Used by attachKafkaError to
// route per-cell compile errors to the correct Form.Item.  Synchronised
// with internal/ingestion/kafka_filter.go::CompileKafkaProgram (the
// backend always wraps with `kafka mapping.<name> 编译失败`).
export const KAFKA_MAPPING_FIELD: Record<string, string> = {
  alertname:     'kafka_map_alertname',
  severity:      'kafka_map_severity',
  fingerprint:   'kafka_map_fingerprint',
  summary:       'kafka_map_summary',
  description:   'kafka_map_description',
  starts_at:     'kafka_map_starts_at',
  ends_at:       'kafka_map_ends_at',
  status_path:   'kafka_map_status_path',
  resolved_when: 'kafka_map_resolved_when',
}

// attachKafkaError pulls the per-cell name out of a backend
// mapping/filter compile error message (e.g. `kafka mapping.starts_at
// 编译失败：unrecognized character: U+0040 '@' (1:1) | @timestamp`) and
// sets it as the `errors` of the matching Form.Item, so the operator
// sees the red helper text directly under the input that needs fixing
// instead of a top-level toast.
//
// Returns true when the error was successfully routed to a field; the
// caller should fall back to a global `message.error` only when this
// returns false.
export function attachKafkaError(form: FormInstance<DataSourceFormShape>, raw: string): boolean {
  if (!raw) return false
  if (/^kafka\s+filter\s+编译失败/.test(raw)) {
    form.setFields([{ name: 'kafka_filter', errors: [raw] }])
    return true
  }
  const m = raw.match(/kafka\s+mapping\.([a-zA-Z0-9_.]+)\s+编译失败/)
  if (!m) return false
  const path = m[1]
  const head = path.split('.')[0]
  if (head === 'labels' || head === 'annotations') {
    const listKey = head === 'labels' ? 'kafka_labels' : 'kafka_annotations'
    const key = path.slice(head.length + 1)
    const list = (form.getFieldValue(listKey) || []) as Array<{ key?: string; path?: string }>
    const idx = list.findIndex((r) => r?.key === key)
    if (idx < 0) return false
    form.setFields([{ name: [listKey, idx, 'path'], errors: [raw] }])
    return true
  }
  const dst = KAFKA_MAPPING_FIELD[head] as keyof DataSourceFormShape | undefined
  if (!dst) return false
  form.setFields([{ name: dst, errors: [raw] }])
  return true
}

// NGINX_GATEWAY_SAMPLE is a fully sanitized Higress-shape access-log
// payload that operators can run end-to-end against the recipe that
// `kindDefaults('kafka')` pre-fills.  Designed to:
//
//  - Use only RFC 5737/3849 reserved IPs / domains so we never embed
//    real customer traffic into a public sample.  Domain is
//    `nginx.example.com`, upstream IPs sit in 10.0.0.0/8 (private),
//    client IPs in 198.51.100.0/24 and 203.0.113.0/24 (TEST-NET).
//  - Demonstrate `normalize_path` by including a
//    `/api/v1/users/12345/orders` REST id segment, so the dry-run
//    output immediately shows the `12345` being rewritten to `{id}`
//    in the fingerprint and `path_template` label.
//  - Stay JSON-shaped (no escape pyramid), ~25 fields / ~40 readable
//    lines, so the textarea in the modal stays usable for hand-tweaking.
export const NGINX_GATEWAY_SAMPLE: string = JSON.stringify({
  '@timestamp':           '2026-04-22T10:00:00.000Z',
  requested_server_name:  'nginx.example.com',
  route_name:             'nginx.example.com',
  authority:              'nginx.example.com',
  path:                   '/api/v1/users/12345/orders',
  method:                 'POST',
  protocol:               'HTTP/2',
  response_code:          '500',
  response_code_details:  'via_upstream',
  response_flags:         '-',
  response_body:          '{"error":"internal","trace":"abc"}',
  bytes_received:         '512',
  bytes_sent:             '128',
  duration:               '42',
  upstream_host:          '10.0.0.10:8080',
  upstream_cluster:       'outbound|8080||orders.svc.cluster.local',
  upstream_service_time:  '40',
  true_client_ip:         '198.51.100.1',
  'x-forwarded-for':      '198.51.100.1, 203.0.113.10',
  downstream_remote_address: '203.0.113.10:44734',
  downstream_local_address:  '10.0.0.20:443',
  start_time:             '2026-04-22T10:00:00.000Z',
  request_id:             '00000000-0000-0000-0000-000000000001',
  user_agent:             'curl/8.4.0',
  response_headers: {
    status:         '500',
    'content-type': 'application/json; charset=utf-8',
    date:           'Wed, 22 Apr 2026 10:00:00 GMT',
  },
  kubernetes: {
    namespace: 'ingress-system',
    pod: {
      name: 'nginx-gateway-abcde',
      uid:  '00000000-0000-0000-0000-000000000099',
    },
    container: { name: 'nginx-gateway' },
    labels:    { app: 'nginx-gateway' },
  },
}, null, 2)
