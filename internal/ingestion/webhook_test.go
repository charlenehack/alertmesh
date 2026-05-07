package ingestion

import (
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

func TestWebhookAdapter_OpenSearchStyleFlatBody(t *testing.T) {
	payload := []byte(`{
  "monitor_name": "HighErrorRate",
  "severity": "1",
  "monitor_id": "abc-123",
  "trigger_name": "t1",
  "period_start": 1700000000000,
  "error": "too many 5xx"
}`)
	m := WebhookMapping{
		AlertnamePath:   "monitor_name",
		SeverityPath:    "severity",
		FingerprintPath: "monitor_id",
		SummaryPath:     "trigger_name",
		DescriptionPath: "error",
		StartsAtPath:    "period_start",
		LabelPaths: map[string]string{
			"trigger": "trigger_name",
		},
	}
	a := NewWebhookAdapter("opensearch-prod", m)
	alerts, err := a.Adapt(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("len=%d", len(alerts))
	}
	ra := alerts[0]
	if ra.Labels["alertname"] != "HighErrorRate" {
		t.Fatalf("alertname=%q", ra.Labels["alertname"])
	}
	if ra.Labels["trigger"] != "t1" {
		t.Fatalf("trigger=%q", ra.Labels["trigger"])
	}
	if ra.Fingerprint != "abc-123" {
		t.Fatalf("fp=%q", ra.Fingerprint)
	}
	if ra.Annotations["summary"] != "t1" {
		t.Fatalf("summary=%q", ra.Annotations["summary"])
	}
	if ra.Annotations["description"] != "too many 5xx" {
		t.Fatalf("desc=%q", ra.Annotations["description"])
	}
	if ra.StartsAt.UnixMilli() != 1700000000000 {
		t.Fatalf("startsAt=%v", ra.StartsAt)
	}
}

func TestWebhookMappingFromJSON_Empty(t *testing.T) {
	m, err := WebhookMappingFromJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.AlertnamePath != "" {
		t.Fatalf("expected empty, got %#v", m)
	}
}

func TestParseEventTime_StringRFC3339(t *testing.T) {
	r := gjson.Parse(`{"t":"2024-06-01T12:00:00Z"}`)
	got := parseEventTime(r.Get("t"), time.Unix(0, 0).UTC())
	if got.Year() != 2024 || got.Month() != time.June || got.Day() != 1 {
		t.Fatalf("got %v", got)
	}
}
