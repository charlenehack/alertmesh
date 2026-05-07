import { http, ApiError } from './request'
import { useAuthStore } from '../store/auth'
import type {
  DataSource,
  DataSourceTestResult,
  PromQueryResponse,
} from '../types'

// ─── Data sources (admin-only CRUD) ───────────────────────────────────────────
//
// Secrets in the request body must already be wire-encrypted via
// `api/crypto.ts::encryptSecret` and prefixed with "ENC:".  Blank strings or
// the literal "******" mean "keep existing ciphertext on the server".

export interface DataSourceWritePayload {
  name?: string
  kind?: DataSource['kind']
  description?: string
  is_enabled?: boolean
  is_default?: boolean
  // Log sources only: enables manual AI on incidents from this source.
  ai_enabled?: boolean
  // Optional: enqueue AI when a new incident is created (default false server-side).
  ai_auto_trigger?: boolean
  endpoint?: string
  config?: Record<string, unknown>
  // Map of secret name → "ENC:<base64-rsa>" / "" / "******".  Keys vary by kind:
  //   prometheus: { token?, password? }
  //   k8s:        { token? }
  //   opensearch: { password? }
  //   kafka:      { sasl_password? }
  secrets?: Record<string, string>
}

export const getDataSources = (kind?: DataSource['kind']) =>
  http.get<DataSource[]>('/data-sources', kind ? { params: { kind } } : undefined)

export const createDataSource = (data: DataSourceWritePayload) =>
  http.post<DataSource>('/data-sources', data)

export const updateDataSource = (id: string, data: DataSourceWritePayload) =>
  http.put<DataSource>(`/data-sources/${id}`, data)

export const deleteDataSource = (id: string) =>
  http.delete<null>(`/data-sources/${id}`)

export const setDefaultDataSource = (id: string) =>
  http.post<{ id: string; status: string }>(`/data-sources/${id}/set-default`)

// `id` may be the literal "new" when validating an unsaved row.
export const testDataSource = (id: string, body: DataSourceWritePayload) =>
  http.post<DataSourceTestResult>(`/data-sources/${id || 'new'}/test`, body)

// Dry-run a single sample payload through this row's filter + mapping.
// Used by the Kafka data-source drawer's "测试" button so operators can
// iterate on the expression / paths without producing into Kafka.  When
// `config` is provided it overlays the in-flight (unsaved) edits on top
// of the stored row; omit it to test the row exactly as persisted.
export interface KafkaTestMessageBody {
  sample: string | Record<string, unknown>
  config?: {
    filter?: string
    mapping?: Record<string, unknown>
  }
}

export interface KafkaTestMessageResult {
  kept: boolean
  drop_reason?: string
  raw_alert?: {
    source: string
    fingerprint: string
    labels: Record<string, string>
    annotations: Record<string, string>
    starts_at: string
    ends_at?: string
    status: string
  }
  debug: {
    filter_eval: boolean | null
    resolved: boolean
    mapping_resolved: Record<string, string>
  }
}

export const testKafkaMessage = (id: string, body: KafkaTestMessageBody) =>
  http.post<KafkaTestMessageResult>(`/data-sources/${id || 'new'}/test-message`, body)

// ─── Prometheus query proxy ───────────────────────────────────────────────────
//
// The backend forwards Prometheus's response **verbatim** (no envelope), so we
// can't go through the standard `http` helper which expects {code,message,data}.
// Hand-roll a small fetch that injects the bearer token but parses the raw
// Prometheus JSON.  The shape is identical to what Prometheus's own /api/v1/*
// endpoints return, so any Prometheus client snippet drops in unchanged.

const BASE = '/api/v1'

async function rawProm(path: string, params: Record<string, string>): Promise<PromQueryResponse> {
  const qs = new URLSearchParams(params).toString()
  const token = useAuthStore.getState().token
  const res = await fetch(`${BASE}${path}?${qs}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  })
  if (res.status === 401) {
    useAuthStore.getState().logout()
    window.location.href = '/login'
    throw new ApiError(401, 401, 'Unauthorized')
  }
  let json: PromQueryResponse
  try {
    json = await res.json()
  } catch {
    throw new ApiError(res.status, -1, `Prometheus proxy returned non-JSON (${res.status})`)
  }
  if (!res.ok || json.status !== 'success') {
    throw new ApiError(res.status, -1, json.error || `Prometheus error (${res.status})`)
  }
  return json
}

export const promQuery = (id: string, query: string, time?: string) =>
  rawProm(`/data-sources/${id}/prom/query`, time ? { query, time } : { query })

export const promQueryRange = (
  id: string,
  query: string,
  startSec: number,
  endSec: number,
  step: string,
) =>
  rawProm(`/data-sources/${id}/prom/query_range`, {
    query,
    start: String(startSec),
    end: String(endSec),
    step,
  })

export const promLabels = (id: string) =>
  rawProm(`/data-sources/${id}/prom/labels`, {})
