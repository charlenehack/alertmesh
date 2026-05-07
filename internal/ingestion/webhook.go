package ingestion

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// WebhookMapping defines gjson paths (tidwall/gjson syntax, e.g. "monitor.name",
// "results.0.key") from the signed webhook JSON body into RawAlert fields.
// Stored per row in webhook_sources.mapping — see docs/log-alert-denoising.md.
type WebhookMapping struct {
	AlertnamePath   string            `json:"alertname_path"`
	SeverityPath    string            `json:"severity_path"`
	ServicePath     string            `json:"service_path,omitempty"`
	DescriptionPath string            `json:"description_path,omitempty"`
	SummaryPath     string            `json:"summary_path,omitempty"`
	StartsAtPath    string            `json:"starts_at_path,omitempty"`
	FingerprintPath string            `json:"fingerprint_path,omitempty"`
	LabelPaths      map[string]string `json:"label_paths,omitempty"`
}

// WebhookAdapter normalises generic webhook payloads using gjson path mappings.
type WebhookAdapter struct {
	source  string
	mapping WebhookMapping
}

func NewWebhookAdapter(source string, mapping WebhookMapping) *WebhookAdapter {
	return &WebhookAdapter{source: source, mapping: mapping}
}

func (a *WebhookAdapter) Name() string { return a.source }

func (a *WebhookAdapter) Adapt(payload []byte) ([]RawAlert, error) {
	if !gjson.ValidBytes(payload) {
		return nil, errors.New("invalid JSON payload")
	}

	alertname := strings.TrimSpace(jsonScalarString(payload, a.mapping.AlertnamePath))
	severity := strings.TrimSpace(jsonScalarString(payload, a.mapping.SeverityPath))
	if alertname == "" || severity == "" {
		return nil, errors.New("missing required fields: alertname or severity (check alertname_path / severity_path)")
	}

	labels := map[string]string{
		"alertname": alertname,
		"severity":  severity,
		"source":    a.source,
	}
	if svc := strings.TrimSpace(jsonScalarString(payload, a.mapping.ServicePath)); svc != "" {
		labels["service"] = svc
	}
	for lk, jp := range a.mapping.LabelPaths {
		if lk == "" || jp == "" {
			continue
		}
		if v := strings.TrimSpace(jsonScalarString(payload, jp)); v != "" {
			labels[lk] = v
		}
	}

	annotations := make(map[string]string)
	if desc := strings.TrimSpace(jsonScalarString(payload, a.mapping.DescriptionPath)); desc != "" {
		annotations["description"] = desc
	}
	if sum := strings.TrimSpace(jsonScalarString(payload, a.mapping.SummaryPath)); sum != "" {
		annotations["summary"] = sum
	}

	startsAt := time.Now().UTC()
	if p := strings.TrimSpace(a.mapping.StartsAtPath); p != "" {
		startsAt = parseEventTime(gjson.GetBytes(payload, p), startsAt)
	}

	fp := strings.TrimSpace(jsonScalarString(payload, a.mapping.FingerprintPath))
	if fp == "" {
		fp = ComputeFingerprint(labels)
	}

	return []RawAlert{{
		Source:      a.source,
		Fingerprint: fp,
		Labels:      labels,
		Annotations: annotations,
		StartsAt:    startsAt,
		Status:      "firing",
		RawPayload:  append([]byte(nil), payload...),
	}}, nil
}

// jsonScalarString returns a printable string for gjson paths (strings, numbers, bools).
func jsonScalarString(payload []byte, path string) string {
	if path == "" {
		return ""
	}
	r := gjson.GetBytes(payload, path)
	if !r.Exists() || r.Type == gjson.Null {
		return ""
	}
	switch r.Type {
	case gjson.String:
		return r.Str
	case gjson.Number:
		// Preserve integers without scientific notation where possible.
		if f := r.Float(); f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10)
		}
		return r.Raw
	case gjson.True:
		return "true"
	case gjson.False:
		return "false"
	default:
		// Objects / arrays: compact JSON for fingerprint-like fields.
		if len(r.Raw) > 4096 {
			return r.Raw[:4096] + "…"
		}
		return r.Raw
	}
}

func parseEventTime(r gjson.Result, fallback time.Time) time.Time {
	if !r.Exists() || r.Type == gjson.Null {
		return fallback
	}
	switch r.Type {
	case gjson.Number:
		n := r.Int()
		if n > 9999999999 { // epoch millis
			return time.UnixMilli(n).UTC()
		}
		if n > 0 {
			return time.Unix(n, 0).UTC()
		}
	case gjson.String:
		s := strings.TrimSpace(r.Str)
		if s == "" {
			return fallback
		}
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, s); err == nil {
				return t.UTC()
			}
		}
	}
	return fallback
}

// WebhookMappingFromJSON parses mapping bytes; returns zero struct on empty input.
func WebhookMappingFromJSON(b []byte) (WebhookMapping, error) {
	if len(strings.TrimSpace(string(b))) == 0 {
		return WebhookMapping{}, nil
	}
	var m WebhookMapping
	if err := json.Unmarshal(b, &m); err != nil {
		return WebhookMapping{}, err
	}
	return m, nil
}
