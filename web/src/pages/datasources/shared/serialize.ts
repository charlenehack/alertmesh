// Form ⇄ DataSource row ⇄ API payload converters.  Centralised so the
// orchestrator only knows about the resulting payload shape, and a new
// kind requires touching only this file plus its <Form/> component.

import type { DataSource, K8sEventKind } from '../../../types'
import type { DataSourceWritePayload } from '../../../api/datasources'
import { encryptSecret } from '../../../api/crypto'
import { isAIEligibleKind } from './constants'
import { kafkaMappingFromForm } from './kafkaMapping'
import { SECRET_MASK } from './constants'
import type { DataSourceFormShape } from './types'

// arrToCSV / csvToArr keep the DB shape (`string[]`) and the form shape
// (`"prod, infra, monitoring"`) in step.  Trim + drop empties so a
// trailing comma never produces an empty namespace match.
export function arrToCSV(v: unknown): string {
  if (!Array.isArray(v)) return ''
  return v.filter((x) => typeof x === 'string').join(', ')
}

export function csvToArr(v: string | undefined): string[] {
  if (!v) return []
  return v.split(',').map((s) => s.trim()).filter(Boolean)
}

// clampConcurrency mirrors the backend's [1,32] guard for
// `consumer_concurrency` (see internal/router/data_sources.go).  We do
// the clamp client-side too so a stale `undefined` from an old form
// payload always serialises to a healthy default of 1 rather than 0/NaN.
export function clampConcurrency(v: number | undefined): number {
  if (typeof v !== 'number' || !Number.isFinite(v)) return 1
  const n = Math.floor(v)
  if (n < 1) return 1
  if (n > 32) return 32
  return n
}

// rowToForm converts a stored DataSource row into the flat form shape
// the drawer renders.  Secrets are reduced to the SECRET_MASK so the
// "leave blank to keep" UX works without ever exposing ciphertext.
export function rowToForm(row: DataSource): DataSourceFormShape {
  const cfg = (row.config || {}) as Record<string, unknown>
  const populated = (key: string) => row.secret_keys.includes(key) ? SECRET_MASK : undefined
  const asString = (v: unknown): string | undefined => typeof v === 'string' ? v : undefined
  const asBool = (v: unknown): boolean => v === true
  const asNumber = (v: unknown, fallback: number): number =>
    typeof v === 'number' && Number.isFinite(v) ? v : fallback

  const base: DataSourceFormShape = {
    name: row.name,
    kind: row.kind,
    description: row.description,
    endpoint: row.endpoint,
    is_enabled: row.is_enabled,
    is_default: row.is_default,
    ai_enabled: row.ai_enabled,
    ai_auto_trigger: !!row.ai_auto_trigger,
  }

  switch (row.kind) {
    case 'prometheus':
      return {
        ...base,
        prom_auth_type: (cfg.auth_type as 'none' | 'basic' | 'bearer' | undefined) || 'none',
        prom_username:  asString(cfg.username),
        prom_password:  populated('password'),
        prom_token:     populated('token'),
        prom_tls_skip:  asBool(cfg.tls_insecure_skip_verify),
      }
    case 'k8s':
      return {
        ...base,
        k8s_in_cluster:                 asBool(cfg.in_cluster),
        k8s_token:                      populated('token'),
        k8s_tls_skip:                   asBool(cfg.tls_insecure_skip_verify),
        k8s_watched_namespaces:         arrToCSV(cfg.watched_namespaces),
        k8s_ignored_namespaces:         arrToCSV(cfg.ignored_namespaces),
        k8s_ignored_pods_re:            asString(cfg.ignored_pods_re),
        k8s_events:                     (cfg.events as K8sEventKind[]) || [],
        k8s_mute_seconds:               asNumber(cfg.mute_seconds, 1800),
        k8s_ignore_restart_count_above: asNumber(cfg.ignore_restart_count_above, 20),
        k8s_pending_threshold_seconds:  asNumber(cfg.pending_threshold_seconds, 300),
      }
    case 'opensearch':
    case 'elastic':
      return {
        ...base,
        os_username:              asString(cfg.username),
        os_password:              populated('password'),
        os_index:                 asString(cfg.index),
        os_query:                 typeof cfg.query === 'string'
          ? cfg.query
          : (cfg.query ? JSON.stringify(cfg.query, null, 2) : ''),
        os_watermark_field:       asString(cfg.watermark_field) || '@timestamp',
        os_poll_interval_seconds: asNumber(cfg.poll_interval_seconds, 30),
        os_lookback_seconds:      asNumber(cfg.lookback_seconds, 300),
        os_tls_skip:              asBool(cfg.tls_insecure_skip_verify),
        os_consumer_concurrency:  asNumber(cfg.consumer_concurrency, 1),
      }
    case 'kafka': {
      const m = (cfg.mapping || {}) as Record<string, unknown>
      const objToPairs = (raw: unknown): { key: string; path: string }[] => {
        if (!raw || typeof raw !== 'object') return []
        return Object.entries(raw as Record<string, unknown>)
          .filter(([, v]) => typeof v === 'string' && (v as string).length > 0)
          .map(([k, v]) => ({ key: k, path: v as string }))
      }
      return {
        ...base,
        kafka_topic:           asString(cfg.topic),
        kafka_group_id:        asString(cfg.group_id),
        kafka_sasl_mechanism:  (cfg.sasl_mechanism as DataSourceFormShape['kafka_sasl_mechanism']) || '',
        kafka_sasl_user:       asString(cfg.sasl_user),
        kafka_sasl_password:   populated('sasl_password'),
        kafka_tls_enabled:     asBool(cfg.tls_enabled),
        kafka_tls_skip:        asBool(cfg.tls_insecure_skip_verify),
        kafka_max_per_second:  asNumber(cfg.max_per_second, 0),
        kafka_consumer_concurrency: asNumber(cfg.consumer_concurrency, 1),
        kafka_filter:          asString(cfg.filter) ?? '',
        kafka_map_alertname:   asString(m.alertname) ?? 'alertname',
        kafka_map_severity:    asString(m.severity) ?? 'severity',
        kafka_map_fingerprint: asString(m.fingerprint) ?? '',
        kafka_map_starts_at:   asString(m.starts_at) ?? '',
        kafka_map_ends_at:     asString(m.ends_at) ?? '',
        kafka_map_summary:     asString(m.summary) ?? '',
        kafka_map_description: asString(m.description) ?? '',
        kafka_map_status_path: asString(m.status_path) ?? '',
        kafka_map_resolved_when: asString(m.resolved_when) ?? '',
        kafka_labels:          objToPairs(m.labels),
        kafka_annotations:     objToPairs(m.annotations),
      }
    }
  }
}

// formToPayload is the inverse: serialise the form shape into the API
// write payload, encrypting any secret fields with the system RSA key
// before they leave the browser.
export async function formToPayload(v: DataSourceFormShape): Promise<DataSourceWritePayload> {
  const base: DataSourceWritePayload = {
    name: v.name,
    kind: v.kind,
    description: v.description || '',
    is_enabled: v.is_enabled,
    is_default: v.is_default,
    // Server enforces that this is false unless kind is on the AI-eligible
    // whitelist (kafka / opensearch / elastic).  We additionally hide the
    // toggle for unsupported kinds, but defensively strip it here so a
    // stale form value can't smuggle a true through.
    ai_enabled: isAIEligibleKind(v.kind) ? !!v.ai_enabled : false,
    ai_auto_trigger: isAIEligibleKind(v.kind) && !!v.ai_enabled ? !!v.ai_auto_trigger : false,
    endpoint: v.endpoint || '',
  }

  switch (v.kind) {
    case 'prometheus': {
      const cfg: Record<string, unknown> = {
        auth_type:                v.prom_auth_type || 'none',
        tls_insecure_skip_verify: !!v.prom_tls_skip,
      }
      if (v.prom_username) cfg.username = v.prom_username
      const secrets: Record<string, string> = {}
      if (v.prom_token)    secrets.token    = await encryptSecret(v.prom_token)
      if (v.prom_password) secrets.password = await encryptSecret(v.prom_password)
      return { ...base, config: cfg, secrets }
    }
    case 'k8s': {
      const cfg: Record<string, unknown> = {
        in_cluster:                  !!v.k8s_in_cluster,
        tls_insecure_skip_verify:    !!v.k8s_tls_skip,
        watched_namespaces:          csvToArr(v.k8s_watched_namespaces),
        ignored_namespaces:          csvToArr(v.k8s_ignored_namespaces),
        ignored_pods_re:             v.k8s_ignored_pods_re || '',
        events:                      v.k8s_events || [],
        mute_seconds:                v.k8s_mute_seconds ?? 1800,
        ignore_restart_count_above:  v.k8s_ignore_restart_count_above ?? 20,
        pending_threshold_seconds:   v.k8s_pending_threshold_seconds ?? 300,
      }
      const secrets: Record<string, string> = {}
      if (v.k8s_token) secrets.token = await encryptSecret(v.k8s_token)
      return { ...base, config: cfg, secrets }
    }
    case 'opensearch':
    case 'elastic': {
      const cfg: Record<string, unknown> = {
        username:                 v.os_username || '',
        index:                    v.os_index || '',
        query:                    v.os_query || '',
        watermark_field:          v.os_watermark_field || '@timestamp',
        poll_interval_seconds:    v.os_poll_interval_seconds ?? 30,
        lookback_seconds:         v.os_lookback_seconds ?? 300,
        tls_insecure_skip_verify: !!v.os_tls_skip,
        consumer_concurrency:     clampConcurrency(v.os_consumer_concurrency),
      }
      const secrets: Record<string, string> = {}
      if (v.os_password) secrets.password = await encryptSecret(v.os_password)
      return { ...base, config: cfg, secrets }
    }
    case 'kafka': {
      const cfg: Record<string, unknown> = {
        topic:                    v.kafka_topic || '',
        group_id:                 v.kafka_group_id || '',
        sasl_mechanism:           v.kafka_sasl_mechanism || '',
        sasl_user:                v.kafka_sasl_user || '',
        tls_enabled:              !!v.kafka_tls_enabled,
        tls_insecure_skip_verify: !!v.kafka_tls_skip,
        max_per_second:           v.kafka_max_per_second ?? 0,
        consumer_concurrency:     clampConcurrency(v.kafka_consumer_concurrency),
        filter:                   v.kafka_filter || '',
        mapping:                  kafkaMappingFromForm(v),
      }
      const secrets: Record<string, string> = {}
      if (v.kafka_sasl_password) secrets.sasl_password = await encryptSecret(v.kafka_sasl_password)
      return { ...base, config: cfg, secrets }
    }
  }
}
