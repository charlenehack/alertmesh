package ingestion

// AlertmanagerV2Adapter implements the wire format Prometheus uses when it is
// configured with `alerting.alertmanagers.static_configs.targets: [<host>]`
// and posts directly to `POST /api/v2/alerts` — i.e. **alertmesh acting as
// Alertmanager**, no real Alertmanager in the loop.
//
// This is intentionally NOT the same format as the existing
// `/api/v1/alerts/alertmanager` webhook: that endpoint accepts the *grouped*
// notification body Alertmanager itself emits to downstream webhook
// receivers (`{status, alerts: [...], groupLabels: {...}}`).  The
// `/api/v2/alerts` API instead accepts the *raw* PostableAlerts list that
// Prometheus's notifier sends upstream — a top-level JSON array of alert
// objects, no wrapper.
//
// Reference: Alertmanager OpenAPI v2 — components.schemas.postableAlert,
// `POST /api/v2/alerts` accepts `postableAlerts` (an array).
//
// Distinguishing "firing" from "resolved": Prometheus does NOT carry an
// explicit `status` in this format; it signals resolution by setting
// `endsAt` to the resolution time (in the past or imminent future) instead
// of the zero value `0001-01-01T00:00:00Z`.  We mirror what Alertmanager
// itself does: `endsAt > now()` ⇒ firing; `endsAt ≤ now()` ⇒ resolved.

import (
	"encoding/json"
	"fmt"
	"time"
)

type AlertmanagerV2Adapter struct{}

func NewAlertmanagerV2Adapter() *AlertmanagerV2Adapter { return &AlertmanagerV2Adapter{} }

func (a *AlertmanagerV2Adapter) Name() string { return "prometheus" }

// postableAlertV2 mirrors the Alertmanager v2 schema field-for-field; only
// the bits we need are typed, the rest is allowed via standard Go JSON
// laxness (extra keys ignored).
type postableAlertV2 struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
}

func (a *AlertmanagerV2Adapter) Adapt(payload []byte) ([]RawAlert, error) {
	// Fast-path the common case: a JSON array.  Prometheus always sends an
	// array even when it only has one alert, so we don't need to support
	// the singleton-object form.
	var items []postableAlertV2
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, fmt.Errorf("alertmanager-v2: invalid payload (expected JSON array of postableAlert): %w", err)
	}
	if len(items) == 0 {
		// Empty arrays are legal in Prometheus (e.g. the very first scrape
		// has no firing alerts) — accept silently rather than 400 so the
		// notifier's success counter increments cleanly.
		return nil, nil
	}

	now := time.Now().UTC()
	out := make([]RawAlert, 0, len(items))
	for i, it := range items {
		if it.Labels == nil {
			it.Labels = map[string]string{}
		}
		// alertname is the one field Alertmanager actually requires; the
		// engine downstream uses it for routing / aggregation, and a
		// missing alertname usually means a misconfigured Prometheus rule.
		if it.Labels["alertname"] == "" {
			return nil, fmt.Errorf("alertmanager-v2: item[%d] missing required label `alertname`", i)
		}

		// Tag the source so dispatcher / AI / audit can tell this row came
		// in via the direct Prometheus path (vs the relayed Alertmanager
		// webhook).  Don't overwrite an explicit user-set `source` label.
		if _, ok := it.Labels["source"]; !ok {
			it.Labels["source"] = "prometheus"
		}
		// Fill severity if Prometheus didn't set one — keeps the routing
		// engine happy without forcing every rule to carry it.
		if it.Labels["severity"] == "" {
			it.Labels["severity"] = "warning"
		}

		startsAt := it.StartsAt
		if startsAt.IsZero() {
			startsAt = now
		}

		// Status inference: endsAt == 0 OR endsAt is in the future ⇒ still
		// firing; endsAt ≤ now ⇒ resolved.  This is exactly the rule
		// Alertmanager uses internally (see api/v2/api.go::AddAlerts).
		var endsAt *time.Time
		status := "firing"
		switch {
		case it.EndsAt.IsZero():
			// firing, no end ⇒ leave nil
		case it.EndsAt.After(now):
			t := it.EndsAt
			endsAt = &t
		default:
			t := it.EndsAt
			endsAt = &t
			status = "resolved"
		}

		ann := it.Annotations
		if ann == nil {
			ann = map[string]string{}
		}
		if it.GeneratorURL != "" {
			ann["generator_url"] = it.GeneratorURL
		}

		out = append(out, RawAlert{
			Source:      "prometheus",
			Fingerprint: ComputeFingerprint(it.Labels),
			Labels:      it.Labels,
			Annotations: ann,
			StartsAt:    startsAt,
			EndsAt:      endsAt,
			Status:      status,
			RawPayload:  payload,
		})
	}

	return out, nil
}
