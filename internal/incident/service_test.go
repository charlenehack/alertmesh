package incident

import (
	"strings"
	"testing"

	"github.com/kuzane/alertmesh/internal/engine"
	"github.com/kuzane/alertmesh/internal/ingestion"
)

// TestBuildNotificationBody is a contract test for the rendered IM /
// email message body.  We pin four behaviours:
//
//  1. Legacy Prometheus-shape alerts (description-only) keep working
//     and don't gain stray empty sections; alertname / severity are
//     deliberately absent from body — they live in the title bar
//     adapters render at the top of every IM card.
//  2. The full Kafka shape surfaces every annotation including the
//     auto-injected kafka_filter_expr; labels like `route_name` no
//     longer appear in the body (they only contribute to dedup /
//     routing) — this is intentional after the "维度 → 消息源" rewrite.
//  3. The 消息源 line renders kind alone when dsName is empty
//     (legacy alertmanager / prometheus push) and "kind : dsName"
//     when both are present (Kafka / OpenSearch / Elastic registry rows).
//  4. Per-value and total-body caps both engage on oversize input.
func TestBuildNotificationBody(t *testing.T) {
	t.Run("legacy_minimal", func(t *testing.T) {
		group := engine.AlertGroup{
			Severity: "P3",
			Labels: map[string]string{
				"alertname": "DiskHigh",
				"service":   "node-1",
				"namespace": "infra",
			},
			Alerts: []ingestion.RawAlert{{
				Source: "alertmanager",
				Annotations: map[string]string{
					"description": "disk usage 86%",
				},
			}},
		}
		body := buildNotificationBody(group, "alertmanager", "")

		// alertname / severity now live in the title only.  Body must
		// NOT echo them back.
		mustNotContain(t, body, "**告警名称:**")
		mustNotContain(t, body, "**告警级别:**")
		// 告警数量 stays — the title doesn't carry the count.
		mustContain(t, body, "**告警数量:** 1")
		// 消息源 collapses to just the kind when no dsName is provided.
		mustContain(t, body, "**消息源:** alertmanager")
		mustNotContain(t, body, " : ")
		// The legacy 维度 KV-fanout is gone — service / namespace
		// labels no longer leak into the IM body.
		mustNotContain(t, body, "**维度:**")
		mustNotContain(t, body, "- service:")
		mustNotContain(t, body, "- namespace:")
		// Description still appears in 详情.
		mustContain(t, body, "**详情:**")
		mustContain(t, body, "disk usage 86%")
		// No 上下文 section because the only annotation was description
		// (which is rendered separately).
		mustNotContain(t, body, "**上下文:**")
	})

	t.Run("kafka_full", func(t *testing.T) {
		group := engine.AlertGroup{
			Severity: "P2",
			Labels: map[string]string{
				"alertname":      "nginx.example.com",
				"route_name":     "nginx.example.com",
				"path_template":  "/api/v1/users/{id}/orders",
				"method":         "POST",
				"code":           "500",
				"true_client_ip": "198.51.100.1",
			},
			Alerts: []ingestion.RawAlert{{
				Source: "kafka",
				Annotations: map[string]string{
					"summary":               "POST /api/v1/users/12345/orders -> 500",
					"description":           `{"error":"internal","trace":"abc"}`,
					"request_id":            "00000000-0000-0000-0000-000000000001",
					"upstream":              "10.0.0.10:8080",
					"response_body":         `{"error":"internal","trace":"abc"}`,
					"response_code_details": "via_upstream",
					"kafka_filter_expr":     `response_body != "-"`,
				},
			}},
		}
		body := buildNotificationBody(group, "kafka", "higress-prod-test1")

		// alertname / severity are in the title, not the body.
		mustNotContain(t, body, "**告警名称:**")
		mustNotContain(t, body, "**告警级别:**")
		mustNotContain(t, body, "- alertname:")

		// Summary line surfaces above the sections.
		mustContain(t, body, "POST /api/v1/users/12345/orders -> 500")

		// 消息源 renders "kind : dsName" when both are provided.
		mustContain(t, body, "**消息源:** kafka : higress-prod-test1")

		// The labels-as-rows section is gone; route_name / method / code
		// belong to dedup + routing, not to the user-facing IM body.
		mustNotContain(t, body, "**维度:**")
		mustNotContain(t, body, "- code: 500")
		mustNotContain(t, body, "- method: POST")
		mustNotContain(t, body, "- route_name:")

		// Context section surfaces every mapped annotation including the
		// auto-injected kafka_filter_expr — this is the bug the fanout
		// rewrite is meant to fix.
		mustContain(t, body, "**上下文:**")
		mustContain(t, body, `- kafka_filter_expr: response_body != "-"`)
		mustContain(t, body, "- request_id: 00000000-0000-0000-0000-000000000001")
		mustContain(t, body, "- upstream: 10.0.0.10:8080")
		mustContain(t, body, "- response_code_details: via_upstream")

		// description is rendered in 详情 and NOT duplicated as a context row.
		if strings.Count(body, `{"error":"internal","trace":"abc"}`) != 2 {
			// 2 occurrences: once in the 上下文 row for response_body, once
			// in the 详情 block (which uses the description annotation that
			// happens to share the same value because the default mapping
			// sets both to response_body).  Anything else means we either
			// double-counted description or dropped one.
			t.Fatalf("expected 2 occurrences of the JSON body, got %d\n%s",
				strings.Count(body, `{"error":"internal","trace":"abc"}`), body)
		}
		mustContain(t, body, "**详情:**")

		// Stable ordering: the 上下文 keys must come out alphabetically
		// regardless of map iteration order (kafka_filter_expr <
		// request_id < response_body < response_code_details < upstream).
		assertOrdered(t, body, []string{
			"- kafka_filter_expr: ",
			"- request_id: ",
			"- response_body: ",
			"- response_code_details: ",
			"- upstream: ",
		})

		// Skipped keys (summary, description) are not in the 上下文 list.
		mustNotContain(t, body, "- summary:")
		mustNotContain(t, body, "- description:")
	})

	t.Run("source_kind_only_when_no_dsname", func(t *testing.T) {
		// Sanity check the fallback contract used by alertmanager /
		// prometheus / webhook ingestion paths that never carry a
		// data_source_id.  resolveSource hands back ("alertmanager", "")
		// in that case and the renderer should emit just the kind.
		group := engine.AlertGroup{
			Severity: "P1",
			Labels:   map[string]string{"alertname": "WhateverAlert"},
			Alerts:   []ingestion.RawAlert{{Source: "alertmanager"}},
		}
		body := buildNotificationBody(group, "alertmanager", "")
		mustContain(t, body, "**消息源:** alertmanager\n")
	})

	t.Run("truncation", func(t *testing.T) {
		// One annotation that blows past the per-value cap.
		bigVal := strings.Repeat("a", 1000)
		// A bunch of medium annotations to push the total past the body cap.
		longAnno := map[string]string{
			"description": strings.Repeat("d", 1500),
			"big":         bigVal,
		}
		for i := 0; i < 20; i++ {
			longAnno["fill_"+string(rune('a'+i))] = strings.Repeat("x", 200)
		}
		group := engine.AlertGroup{
			Severity: "P3",
			Labels:   map[string]string{"alertname": "big"},
			Alerts:   []ingestion.RawAlert{{Source: "kafka", Annotations: longAnno}},
		}
		body := buildNotificationBody(group, "kafka", "test-ds")

		// Per-row cap engaged: bigVal is 1000 chars but only 256 should
		// render before the suffix.
		mustContain(t, body, "- big: "+strings.Repeat("a", notifyValueMaxLen)+" …(共 1000 字)")

		// Total body cap engaged.
		mustContain(t, body, "…(已截断)")
		if rs := []rune(body); len(rs) > notifyBodyMaxLen+len(" …(已截断)") {
			t.Fatalf("body length = %d runes, expected <= %d", len(rs),
				notifyBodyMaxLen+len(" …(已截断)"))
		}
	})
}

func mustContain(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("body missing %q\n--- body ---\n%s\n--- end ---", want, body)
	}
}

func mustNotContain(t *testing.T, body, unwanted string) {
	t.Helper()
	if strings.Contains(body, unwanted) {
		t.Fatalf("body unexpectedly contained %q\n--- body ---\n%s\n--- end ---", unwanted, body)
	}
}

// assertOrdered checks that each prefix appears in `body` in the given
// order (and at least once).  Used to pin alphabetical sort of the kv
// sections regardless of Go's randomised map iteration.
func assertOrdered(t *testing.T, body string, prefixes []string) {
	t.Helper()
	pos := -1
	for _, p := range prefixes {
		i := strings.Index(body, p)
		if i < 0 {
			t.Fatalf("missing prefix %q in body\n%s", p, body)
		}
		if i <= pos {
			t.Fatalf("prefix %q at offset %d should be after previous (%d)\n%s",
				p, i, pos, body)
		}
		pos = i
	}
}
