package ingestion

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RawAlert is the unified internal alert representation after normalisation.
type RawAlert struct {
	Source      string            `json:"source"`      // alertmanager / prometheus / k8s / cloud-rds / kafka / webhook
	Fingerprint string            `json:"fingerprint"` // dedup key
	Labels      map[string]string `json:"labels"`      // must contain alertname, severity, source
	Annotations map[string]string `json:"annotations"` // summary, description, runbook_url
	StartsAt    time.Time         `json:"starts_at"`
	EndsAt      *time.Time        `json:"ends_at"`     // nil means still firing
	Status      string            `json:"status"`      // firing / resolved
	RawPayload  json.RawMessage   `json:"raw_payload"` // original payload for audit
	// DataSourceID is the data_sources.id this alert was ingested through.
	// Empty for adapters that don't yet have a source registry hookup
	// (legacy alertmanager push, ad-hoc webhook).  Threaded through
	// engine.AlertGroup → incident.Service so the AI gate can look up the
	// owning data source's ai_enabled / ai_auto_trigger flags.
	DataSourceID string `json:"data_source_id,omitempty"`
}

// Adapter normalises a source-specific payload into one or more RawAlerts.
type Adapter interface {
	Name() string
	Adapt(payload []byte) ([]RawAlert, error)
}

// ComputeFingerprint generates a SHA256-based fingerprint from sorted label key-value pairs.
func ComputeFingerprint(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
		b.WriteByte('\n')
	}

	h := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("%x", h)
}
