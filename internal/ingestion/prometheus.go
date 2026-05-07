package ingestion

import (
	"encoding/json"
	"fmt"
	"time"
)

// PrometheusAdapter accepts the wrapped {status,alerts:[...]} JSON shape that
// Alertmanager itself emits to *downstream* webhook receivers.  It is wired
// to /api/v1/alerts/prometheus/remote purely as a compatibility alias for the
// /api/v1/alerts/alertmanager endpoint — it does NOT speak the Prometheus
// Remote Write protobuf protocol, and it is NOT what Prometheus's notifier
// posts when configured with `alerting.alertmanagers` (that goes to the
// /api/v2/alerts route handled by AlertmanagerV2Adapter — see
// internal/ingestion/alertmanager_v2.go for the real wire format).
type PrometheusAdapter struct{}

func NewPrometheusAdapter() *PrometheusAdapter { return &PrometheusAdapter{} }

func (a *PrometheusAdapter) Name() string { return "prometheus" }

// promAlert mirrors the alert structure in Prometheus/Alertmanager webhook payloads.
type promAlert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

type promPayload struct {
	Status string      `json:"status"`
	Alerts []promAlert `json:"alerts"`
}

func (a *PrometheusAdapter) Adapt(payload []byte) ([]RawAlert, error) {
	var p promPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("prometheus: invalid payload: %w", err)
	}

	if len(p.Alerts) == 0 {
		return nil, fmt.Errorf("prometheus: no alerts in payload")
	}

	alerts := make([]RawAlert, 0, len(p.Alerts))
	for _, pa := range p.Alerts {
		if pa.Labels == nil {
			pa.Labels = make(map[string]string)
		}
		pa.Labels["source"] = "prometheus"

		if pa.Labels["severity"] == "" {
			pa.Labels["severity"] = "warning"
		}

		fp := pa.Fingerprint
		if fp == "" {
			fp = ComputeFingerprint(pa.Labels)
		}

		var endsAt *time.Time
		if !pa.EndsAt.IsZero() && pa.EndsAt.After(pa.StartsAt) {
			endsAt = &pa.EndsAt
		}

		if pa.Annotations == nil {
			pa.Annotations = make(map[string]string)
		}
		if pa.GeneratorURL != "" {
			pa.Annotations["generator_url"] = pa.GeneratorURL
		}

		alerts = append(alerts, RawAlert{
			Source:      "prometheus",
			Fingerprint: fp,
			Labels:      pa.Labels,
			Annotations: pa.Annotations,
			StartsAt:    pa.StartsAt,
			EndsAt:      endsAt,
			Status:      pa.Status,
			RawPayload:  payload,
		})
	}

	return alerts, nil
}
