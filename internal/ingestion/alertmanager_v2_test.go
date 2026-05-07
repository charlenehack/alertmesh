package ingestion

import (
	"strings"
	"testing"
	"time"
)

// TestAlertmanagerV2Adapter_PrometheusFiringPayload locks down the wire
// contract Prometheus uses when configured with
// `alerting.alertmanagers.static_configs.targets:[<host>]` and posts directly
// to /api/v2/alerts.  The notifier ALWAYS sends a top-level JSON array of
// PostableAlert objects (never a wrapped {alerts:[...]}) and signals
// resolution by setting endsAt to a non-zero past timestamp.
func TestAlertmanagerV2Adapter_PrometheusFiringPayload(t *testing.T) {
	a := NewAlertmanagerV2Adapter()

	body := []byte(`[
		{
			"labels":{"alertname":"HighCPU","severity":"critical","instance":"node-1"},
			"annotations":{"summary":"cpu above 90%"},
			"startsAt":"2026-04-20T10:00:00Z",
			"endsAt":"0001-01-01T00:00:00Z",
			"generatorURL":"http://prom/graph?g0.expr=..."
		},
		{
			"labels":{"alertname":"DiskFull","severity":"warning"},
			"annotations":{"summary":"disk 95%"},
			"startsAt":"2026-04-20T10:01:00Z",
			"endsAt":"2020-01-01T00:00:00Z"
		}
	]`)

	got, err := a.Adapt(body)
	if err != nil {
		t.Fatalf("Adapt returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d alerts, want 2", len(got))
	}

	// Firing case (zero endsAt).
	if got[0].Status != "firing" {
		t.Errorf("alerts[0].Status = %q, want firing", got[0].Status)
	}
	if got[0].EndsAt != nil {
		t.Errorf("alerts[0].EndsAt = %v, want nil for zero endsAt", got[0].EndsAt)
	}
	if got[0].Source != "prometheus" {
		t.Errorf("alerts[0].Source = %q, want prometheus", got[0].Source)
	}
	if got[0].Labels["source"] != "prometheus" {
		t.Errorf(`alerts[0].Labels["source"] = %q, want prometheus`, got[0].Labels["source"])
	}
	if got[0].Annotations["generator_url"] == "" {
		t.Errorf("generatorURL was not promoted to annotations.generator_url")
	}

	// Resolved case (endsAt in the past).
	if got[1].Status != "resolved" {
		t.Errorf("alerts[1].Status = %q, want resolved", got[1].Status)
	}
	if got[1].EndsAt == nil || got[1].EndsAt.IsZero() {
		t.Errorf("alerts[1].EndsAt should be the parsed past timestamp, got %v", got[1].EndsAt)
	}
}

// TestAlertmanagerV2Adapter_EmptyArrayAccepted documents the
// silent-accept behaviour for empty PostableAlerts batches; Prometheus's
// notifier increments its success counter on 2xx so a 400 here would
// trigger an avoidable retry storm.
func TestAlertmanagerV2Adapter_EmptyArrayAccepted(t *testing.T) {
	got, err := NewAlertmanagerV2Adapter().Adapt([]byte(`[]`))
	if err != nil {
		t.Fatalf("empty array rejected: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d alerts, want 0", len(got))
	}
}

// TestAlertmanagerV2Adapter_RejectsAlertmanagerWebhookShape guards against
// confusion between this endpoint and /api/v1/alerts/alertmanager: the
// downstream-receiver shape `{status,alerts:[...]}` is NOT what Prometheus
// posts to /api/v2/alerts and we must reject it loudly so misconfiguration
// surfaces at the edge instead of silently dropping every alert.
func TestAlertmanagerV2Adapter_RejectsAlertmanagerWebhookShape(t *testing.T) {
	body := []byte(`{"status":"firing","alerts":[{"labels":{"alertname":"X"}}]}`)
	_, err := NewAlertmanagerV2Adapter().Adapt(body)
	if err == nil {
		t.Fatal("expected error for wrapped webhook shape, got nil")
	}
	if !strings.Contains(err.Error(), "JSON array") {
		t.Errorf("error message %q should mention `JSON array`", err.Error())
	}
}

// TestAlertmanagerV2Adapter_DefaultsSeverity ensures rules that fire without
// a severity label still flow through the engine instead of being rejected
// downstream by routing rules that key on severity.
func TestAlertmanagerV2Adapter_DefaultsSeverity(t *testing.T) {
	body := []byte(`[{"labels":{"alertname":"NoSeverity"},"annotations":{},"endsAt":"0001-01-01T00:00:00Z"}]`)
	got, err := NewAlertmanagerV2Adapter().Adapt(body)
	if err != nil {
		t.Fatalf("Adapt returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d alerts, want 1", len(got))
	}
	if got[0].Labels["severity"] != "warning" {
		t.Errorf("severity = %q, want warning", got[0].Labels["severity"])
	}
}

// TestAlertmanagerV2Adapter_StartsAtFallback covers the (rare) case where
// Prometheus omits startsAt; we MUST default to now() so the dedup key /
// incident timeline aren't corrupted by a 0001 timestamp.
func TestAlertmanagerV2Adapter_StartsAtFallback(t *testing.T) {
	body := []byte(`[{"labels":{"alertname":"X"},"endsAt":"0001-01-01T00:00:00Z"}]`)
	before := time.Now().Add(-time.Second)
	got, err := NewAlertmanagerV2Adapter().Adapt(body)
	if err != nil {
		t.Fatalf("Adapt returned error: %v", err)
	}
	if got[0].StartsAt.Before(before) {
		t.Errorf("StartsAt should default to ~now, got %v", got[0].StartsAt)
	}
}
