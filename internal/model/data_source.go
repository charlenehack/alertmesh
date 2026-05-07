package model

import (
	"time"

	"gorm.io/datatypes"
)

// DataSource is the unified registry row for an upstream system that
// alertmesh either pulls alerts from (Kafka / OpenSearch / K8s) or queries
// on demand (Prometheus, used by AI tools and the operator-facing PromQL
// Explore page).  The shape is deliberately uniform across kinds so the
// frontend can render one list / one drawer with kind-aware fields, and
// future kinds (云监控 / Loki / …) plug in without table churn.
//
// Two-column secret split:
//
//   - `Config` (jsonb)  – non-secret per-kind structured config; ALWAYS
//     visible in list/get; this is what the UI renders and what
//     `/data-sources/{id}/test` re-uses.
//   - `SecretEnc` (text) – AES-256-GCM ciphertext of a small JSON object
//     holding the actual secrets (token / password / sasl_password /
//     api_key).  NEVER returned by GET; the API only echoes back which
//     keys are populated so the UI can render "leave blank to keep".
//
// The split mirrors the LLM-provider pattern (`api_key` text vs the rest
// of the row); wire-encryption of incoming secrets reuses
// `auth.DecodeClientCipher` and at-rest encryption reuses
// `internal/config.Encrypt` via the existing `cfgcrypto` import path used
// in `router/llm_providers.go`.
//
// Per-kind `Config` schemas (informal — validated in router/data_sources.go):
//
//	prometheus:
//	  { auth_type: "none"|"basic"|"bearer", username, scrape_timeout_seconds }
//	  endpoint = http://prometheus:9090
//
//	k8s:
//	  { in_cluster: bool, ca_cert_pem, watched_namespaces: ["prod","infra"],
//	    ignored_namespaces: ["kube-system"], ignored_pods_re,
//	    events: ["pod_restart","pod_pending","hpa_scale","node_not_ready",
//	             "failed_scheduling"],
//	    mute_seconds: 1800, ignore_restart_count_above: 20,
//	    pending_threshold_seconds: 300 }
//	  endpoint = https://kubernetes.default.svc:6443  (or empty + in_cluster)
//
//	opensearch:
//	  { username, index, query (raw OS DSL), watermark_field,
//	    poll_interval_seconds, lookback_seconds, tls_insecure_skip_verify,
//	    consumer_concurrency } -- int [1,32], default 1; reserved for the
//	                              Phase-4 OpenSearch poller (the row spawns N
//	                              independent watermark-cursor goroutines).
//	  endpoint = https://opensearch.internal:9200
//
//	kafka:
//	  { topic, group_id, sasl_mechanism: "PLAIN"|"SCRAM-SHA-256"|"SCRAM-SHA-512"|"",
//	    sasl_user, tls_enabled, tls_insecure_skip_verify, max_per_second,
//	    consumer_concurrency, -- int [1,32], default 1.  Number of independent
//	                              kafka.Reader goroutines spawned for this row.
//	                              All N readers share the same GroupID so the
//	                              broker auto-distributes partitions; setting N
//	                              higher than the topic partition count just
//	                              parks the extras.
//	    filter,    -- expr-lang boolean expression evaluated against the parsed
//	                  JSON payload; "" or missing == let everything through.
//	                  Example: `level == "ERROR" && env in ["prod","pre"]`.
//	    mapping }  -- gjson path map describing how to project the inbound JSON
//	                  onto a RawAlert.  Required keys (paths into the payload):
//	                    alertname, severity
//	                  Optional scalar keys (each "" disables the field):
//	                    fingerprint, starts_at, ends_at,
//	                    summary, description,
//	                    status_path,    -- payload field that already says "firing"/"resolved"
//	                    resolved_when   -- expr that, when true, marks the alert resolved
//	                  Map keys (output → payload path):
//	                    labels:      { service: "svc", host: "host" }
//	                    annotations: { runbook_url: "runbook" }
//	                  Defaults seeded by migration 000043 mirror the
//	                  Prometheus-shaped JSON so a vanilla
//	                  `{"alertname":"X","severity":"P3","summary":"…"}`
//	                  payload ingests with zero UI configuration.
//	  endpoint = kafka-1:9092,kafka-2:9092
type DataSource struct {
	ID          string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name        string `gorm:"uniqueIndex;not null;type:varchar(128)"         json:"name"`
	Kind        string `gorm:"not null;index;type:varchar(32)"                json:"kind"` // prometheus|k8s|opensearch|kafka
	Description string `gorm:"not null;default:''"                            json:"description"`
	IsEnabled   bool   `gorm:"not null;default:false"                         json:"is_enabled"`
	IsDefault   bool   `gorm:"not null;default:false"                         json:"is_default"`

	// AIEnabled gates whether incidents from this source may use AI at all
	// (manual trigger from the UI + eligibility for optional auto-enqueue).
	// Restricted to log-shaped kinds at the DB-CHECK and router layer.
	AIEnabled bool `gorm:"not null;default:false" json:"ai_enabled"`

	// AIAutoTrigger when true AND AIEnabled: enqueue an ai_tasks row on every
	// new incident create.  When false (the default), operators must click
	// "触发 AI 分析" to spend LLM tokens.  DB CHECK forbids auto without AIEnabled.
	AIAutoTrigger bool `gorm:"not null;default:false" json:"ai_auto_trigger"`

	// Public connection endpoint (URL / brokers / api server) duplicated out
	// of `Config` for fast listing + indexing.  Always non-secret.
	Endpoint string `gorm:"not null;default:''" json:"endpoint"`

	// Per-kind public structured config; see schema notes above.
	Config datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"config"`

	// AES-256-GCM ciphertext of {"token":"…","password":"…",…}.  Marshalled
	// out as `json:"-"` defense-in-depth so we never accidentally return it
	// in a list/get response.  The handler exposes a `secret_keys: [...]`
	// view field so the UI knows which keys are populated.
	SecretEnc string `gorm:"type:text;not null;default:''" json:"-"`

	LastError  *string    `gorm:"type:text"          json:"last_error,omitempty"`
	LastTestAt *time.Time `gorm:""                   json:"last_test_at,omitempty"`
	LastTestOK *bool      `gorm:""                   json:"last_test_ok,omitempty"`

	Timestamps
}

// TableName aligns with migration 34.
func (DataSource) TableName() string { return "data_sources" }

// DataSource kind enum (mirrored in migration 34's CHECK constraint and in
// the frontend `DataSourceKind` type union).
const (
	DataSourceKindPrometheus = "prometheus"
	DataSourceKindK8s        = "k8s"
	DataSourceKindOpenSearch = "opensearch"
	DataSourceKindKafka      = "kafka"
	// DataSourceKindElastic shares the OpenSearch HTTP query DSL + Basic-Auth
	// credential shape; the runtime treats it as a parallel case in every
	// switch on Kind.  Surfaced as a separate kind so operators can wire
	// up Elasticsearch clusters honestly in the UI without overloading
	// "opensearch" semantically.
	DataSourceKindElastic = "elastic"
)

// K8s event-type checkboxes surfaced in the UI.  These are the strings the
// frontend stores into `Config.events[]` and that the K8s connector
// (Phase-3) will use to decide which informers / detectors to wire up.
const (
	K8sEventPodRestart       = "pod_restart"
	K8sEventPodPending       = "pod_pending"
	K8sEventHPAScale         = "hpa_scale"
	K8sEventNodeNotReady     = "node_not_ready"
	K8sEventFailedScheduling = "failed_scheduling"
)
