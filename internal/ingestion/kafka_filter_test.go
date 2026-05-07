package ingestion

import (
	"strings"
	"testing"
	"time"
)

// Table-driven coverage for the Kafka filter+mapping pipeline.  We pin the
// behaviour for the three cases the dispatcher actually relies on:
//
//	keep + map + firing            – default Prometheus-shaped JSON
//	drop  by filter                – expr returns false
//	keep + map + resolved signal   – status_path and resolved_when
//
// plus the two compile-time validation rejections.
func TestKafkaProgramApply(t *testing.T) {
	defaultMapping := KafkaMapping{
		Alertname:   "alertname",
		Severity:    "severity",
		Summary:     "summary",
		Description: "description",
		StartsAt:    "starts_at",
		Labels:      map[string]string{"instance": "instance"},
		Annotations: map[string]string{"runbook_url": "runbook"},
	}

	cases := []struct {
		name        string
		cfg         KafkaFilterConfig
		payload     string
		wantKeep    bool
		wantReason  string
		wantStatus  string
		wantSeverit string
	}{
		{
			name: "default mapping passes Prometheus shape",
			cfg: KafkaFilterConfig{
				Filter:  "",
				Mapping: defaultMapping,
			},
			payload:     `{"alertname":"DiskHigh","severity":"P3","instance":"node-1","summary":"disk 80%","runbook":"https://wiki/runbooks/disk"}`,
			wantKeep:    true,
			wantStatus:  "firing",
			wantSeverit: "P3",
		},
		{
			name: "filter drops mismatched env",
			cfg: KafkaFilterConfig{
				Filter:  `severity == "P0"`,
				Mapping: defaultMapping,
			},
			payload:    `{"alertname":"Foo","severity":"P3"}`,
			wantKeep:   false,
			wantReason: "filter_false",
		},
		{
			name: "missing alertname is dropped",
			cfg: KafkaFilterConfig{
				Mapping: defaultMapping,
			},
			payload:    `{"severity":"P3"}`,
			wantKeep:   false,
			wantReason: "missing_alertname",
		},
		{
			name: "status_path resolved → resolved status",
			cfg: KafkaFilterConfig{
				Mapping: KafkaMapping{
					Alertname:  "alertname",
					Severity:   "severity",
					StatusPath: "state",
				},
			},
			payload:     `{"alertname":"DiskHigh","severity":"P3","state":"resolved"}`,
			wantKeep:    true,
			wantStatus:  "resolved",
			wantSeverit: "P3",
		},
		{
			name: "resolved_when expr triggers resolved",
			cfg: KafkaFilterConfig{
				Mapping: KafkaMapping{
					Alertname:    "alertname",
					Severity:     "severity",
					ResolvedWhen: `level == "INFO"`,
				},
			},
			payload:     `{"alertname":"DiskHigh","severity":"P3","level":"INFO"}`,
			wantKeep:    true,
			wantStatus:  "resolved",
			wantSeverit: "P3",
		},
		{
			name: "bad JSON is dropped",
			cfg: KafkaFilterConfig{
				Mapping: defaultMapping,
			},
			payload:    `not-json`,
			wantKeep:   false,
			wantReason: "bad_json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := CompileKafkaProgram(tc.cfg)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			res, err := prog.Apply([]byte(tc.payload), "kafka", "ds-1")
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if res.Keep != tc.wantKeep {
				t.Fatalf("Keep=%v want %v (reason=%q)", res.Keep, tc.wantKeep, res.Reason)
			}
			if !res.Keep {
				if res.Reason != tc.wantReason {
					t.Fatalf("Reason=%q want %q", res.Reason, tc.wantReason)
				}
				return
			}
			if res.Alert.Status != tc.wantStatus {
				t.Fatalf("Status=%q want %q", res.Alert.Status, tc.wantStatus)
			}
			if res.Alert.Labels["severity"] != tc.wantSeverit {
				t.Fatalf("severity=%q want %q", res.Alert.Labels["severity"], tc.wantSeverit)
			}
			if res.Alert.Source != "kafka" {
				t.Fatalf("source=%q want kafka", res.Alert.Source)
			}
			if res.Alert.DataSourceID != "ds-1" {
				t.Fatalf("data_source_id=%q want ds-1", res.Alert.DataSourceID)
			}
			if tc.wantStatus == "resolved" && res.Alert.EndsAt == nil {
				t.Fatalf("resolved alert should have non-nil EndsAt")
			}
			if tc.wantStatus == "resolved" && res.Alert.EndsAt != nil && res.Alert.EndsAt.After(time.Now().Add(time.Minute)) {
				t.Fatalf("resolved EndsAt unexpectedly in the far future: %v", res.Alert.EndsAt)
			}
		})
	}
}

// TestKafkaApplyForConsumer_NoMappingHits guards the hot-path
// optimisation: ApplyForConsumer must produce the same Alert / Keep /
// Reason as Apply but skip the MappingHits allocation.  The test
// endpoint still uses Apply to populate the debug map.
func TestKafkaApplyForConsumer_NoMappingHits(t *testing.T) {
	cfg := KafkaFilterConfig{
		Mapping: KafkaMapping{
			Alertname:   "alertname",
			Severity:    "severity",
			Summary:     "summary",
			Description: "description",
			Labels:      map[string]string{"instance": "instance"},
			Annotations: map[string]string{"runbook_url": "runbook"},
		},
	}
	prog, err := CompileKafkaProgram(cfg)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	payload := []byte(`{"alertname":"DiskHigh","severity":"P3","instance":"n1","summary":"s","runbook":"r"}`)

	debug, err := prog.Apply(payload, "kafka", "ds")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	consumer, err := prog.ApplyForConsumer(payload, "kafka", "ds")
	if err != nil {
		t.Fatalf("ApplyForConsumer: %v", err)
	}

	if !consumer.Keep || consumer.Alert.Labels["alertname"] != "DiskHigh" {
		t.Fatalf("ApplyForConsumer must produce identical Alert, got %+v", consumer)
	}
	if consumer.MappingHits != nil {
		t.Fatalf("ApplyForConsumer must not allocate MappingHits, got %+v", consumer.MappingHits)
	}
	if debug.MappingHits == nil || len(debug.MappingHits) == 0 {
		t.Fatalf("Apply must still populate MappingHits, got %+v", debug.MappingHits)
	}
	if debug.Alert.Labels["alertname"] != consumer.Alert.Labels["alertname"] ||
		debug.Alert.Status != consumer.Alert.Status ||
		debug.Alert.Annotations["summary"] != consumer.Alert.Annotations["summary"] {
		t.Fatalf("Apply / ApplyForConsumer alert payloads diverged: %+v vs %+v", debug.Alert, consumer.Alert)
	}
}

func TestKafkaCompileRejectsMissingMapping(t *testing.T) {
	_, err := CompileKafkaProgram(KafkaFilterConfig{
		Mapping: KafkaMapping{Severity: "severity"},
	})
	if err == nil || !strings.Contains(err.Error(), "alertname") {
		t.Fatalf("expected alertname-required error, got %v", err)
	}

	_, err = CompileKafkaProgram(KafkaFilterConfig{
		Mapping: KafkaMapping{Alertname: "alertname"},
	})
	if err == nil || !strings.Contains(err.Error(), "severity") {
		t.Fatalf("expected severity-required error, got %v", err)
	}
}

func TestKafkaCompileRejectsBadFilter(t *testing.T) {
	_, err := CompileKafkaProgram(KafkaFilterConfig{
		Filter:  `severity === "P0"`, // expr uses == not ===
		Mapping: KafkaMapping{Alertname: "alertname", Severity: "severity"},
	})
	if err == nil || !strings.Contains(err.Error(), "kafka filter") {
		t.Fatalf("expected filter compile error, got %v", err)
	}
}

// TestStripQuery pins the helper exposed to expr programs.  We only need
// to lock down the contract documented to operators (no allocation when
// `?` is absent, drop everything from `?` onwards otherwise) — the
// compose-with-normalize_path case lives in TestExprFingerprintHigress.
func TestStripQuery(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"", ""},
		{"/foo", "/foo"},
		{"/foo?a=1", "/foo"},
		{"/foo?a=1&b=2", "/foo"},
		{"?a=1", ""},
		{"/wallet/v1/proxy/network", "/wallet/v1/proxy/network"},
	}
	for _, tc := range cases {
		if got := stripQuery(tc.in); got != tc.out {
			t.Errorf("stripQuery(%q) = %q, want %q", tc.in, got, tc.out)
		}
	}
}

// TestNormalizePath table-drives the stock id-pattern set so future
// additions to pathIDPatterns can't silently change the dedup behaviour
// of existing data sources.
func TestNormalizePath(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"", ""},
		{"/", "/"},
		{"/users", "/users"},
		{"/users/12345", "/users/{id}"},
		{"/users/12345/orders", "/users/{id}/orders"},
		{"/v1/users/550e8400-e29b-41d4-a716-446655440000", "/v1/users/{id}"},
		{"/wallet/0xeefba1e63905ef1d7acba5a8513c70307c1ce441", "/wallet/{id}"},
		{"/tx/0xabcdef0123456789abcdef0123", "/tx/{id}"},
		{"/sol/9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM", "/sol/{id}"},
		{"/api/v1/path/no/ids", "/api/v1/path/no/ids"},
		{"/users/123/orders/abc-def", "/users/{id}/orders/abc-def"}, // hyphen breaks both UUID and base64-ish minimums → kept
	}
	for _, tc := range cases {
		if got := normalizePath(tc.in); got != tc.out {
			t.Errorf("normalizePath(%q) = %q, want %q", tc.in, got, tc.out)
		}
	}
}

// TestRegexReplace covers the LRU-cached helper.  We deliberately call
// the function twice with the same pattern to exercise the cache hit
// branch and confirm the second call returns identical output.
func TestRegexReplace(t *testing.T) {
	out, err := regexReplace("/users/12345/orders", `/\d+`, `/{id}`)
	if err != nil {
		t.Fatalf("regexReplace error: %v", err)
	}
	if out != "/users/{id}/orders" {
		t.Fatalf("regexReplace = %q, want /users/{id}/orders", out)
	}
	// Cache hit on second call.
	out2, err := regexReplace("/items/99", `/\d+`, `/{id}`)
	if err != nil || out2 != "/items/{id}" {
		t.Fatalf("regexReplace second call = (%q, %v)", out2, err)
	}
	if _, err := regexReplace("any", `(unbalanced`, "x"); err == nil {
		t.Fatal("expected compile error for invalid pattern")
	}
}

// TestCoalesce documents the "treat dash / null as missing" heuristic
// callers rely on for Higress-style access logs.
func TestCoalesce(t *testing.T) {
	if v := coalesce("-", "", "1.2.3.4"); v != "1.2.3.4" {
		t.Fatalf("coalesce = %v want 1.2.3.4", v)
	}
	if v := coalesce("-", "null", ""); v != "" {
		t.Fatalf("coalesce = %v want empty", v)
	}
	if v := coalesce(nil, "first"); v != "first" {
		t.Fatalf("coalesce = %v want first", v)
	}
}

// TestExprFingerprintHigress is the end-to-end happy-path that pins the
// user-facing recipe: filter on `response_body != "-"`, fingerprint as
// `route_name + "|" + normalize_path(strip_query(path))`.  This is the
// behaviour the §4.1.x README example promises.
func TestExprFingerprintHigress(t *testing.T) {
	prog, err := CompileKafkaProgram(KafkaFilterConfig{
		Filter: `response_body != "-"`,
		Mapping: KafkaMapping{
			Alertname:   "route_name",
			Severity:    `expr: response_code >= "500" ? "P2" : "P3"`,
			Fingerprint: `expr: route_name + "|" + normalize_path(strip_query(path))`,
			Labels: map[string]string{
				"route_name":    "route_name",
				"path_template": `expr: normalize_path(strip_query(path))`,
				"method":        "method",
			},
		},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	payload := []byte(`{
		"route_name": "wallet.onekeycn.com",
		"path": "/wallet/v1/proxy/network",
		"method": "POST",
		"response_code": "200",
		"response_body": "{\"ok\":true}"
	}`)

	res, err := prog.Apply(payload, "kafka", "ds-wallet")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Keep {
		t.Fatalf("expected keep=true, got reason=%q", res.Reason)
	}
	wantFP := "wallet.onekeycn.com|/wallet/v1/proxy/network"
	if res.Alert.Fingerprint != wantFP {
		t.Fatalf("Fingerprint = %q, want %q", res.Alert.Fingerprint, wantFP)
	}
	if res.Alert.Labels["path_template"] != "/wallet/v1/proxy/network" {
		t.Fatalf("path_template label = %q", res.Alert.Labels["path_template"])
	}
	if res.Alert.Labels["severity"] != "P3" {
		t.Fatalf("severity = %q want P3", res.Alert.Labels["severity"])
	}
	// Filter+expr cells should both surface in MappingHits for the
	// test-message endpoint to render.
	if got := res.MappingHits[`expr:expr: route_name + "|" + normalize_path(strip_query(path))`]; got != wantFP {
		t.Fatalf("MappingHits expr fingerprint = %q want %q", got, wantFP)
	}

	// And the same setup with a `?query` suffix should still dedup.
	payload2 := []byte(`{
		"route_name": "wallet.onekeycn.com",
		"path": "/wallet/v1/proxy/network?source=app&v=2",
		"method": "POST",
		"response_code": "200",
		"response_body": "{\"ok\":true}"
	}`)
	res2, err := prog.Apply(payload2, "kafka", "ds-wallet")
	if err != nil || res2.Alert.Fingerprint != wantFP {
		t.Fatalf("query-suffix dedup failed: fp=%q err=%v", res2.Alert.Fingerprint, err)
	}

	// And a REST id segment should normalise.
	payload3 := []byte(`{
		"route_name": "wallet.onekeycn.com",
		"path": "/wallet/v1/users/12345/profile",
		"method": "GET",
		"response_code": "200",
		"response_body": "{\"ok\":true}"
	}`)
	res3, err := prog.Apply(payload3, "kafka", "ds-wallet")
	if err != nil {
		t.Fatalf("rest-id apply: %v", err)
	}
	wantFP3 := "wallet.onekeycn.com|/wallet/v1/users/{id}/profile"
	if res3.Alert.Fingerprint != wantFP3 {
		t.Fatalf("Fingerprint = %q, want %q", res3.Alert.Fingerprint, wantFP3)
	}

	// And response_body == "-" must drop.
	payload4 := []byte(`{
		"route_name": "wallet.onekeycn.com",
		"path": "/wallet/v1/proxy/network",
		"method": "POST",
		"response_code": "200",
		"response_body": "-"
	}`)
	res4, err := prog.Apply(payload4, "kafka", "ds-wallet")
	if err != nil {
		t.Fatalf("filter apply: %v", err)
	}
	if res4.Keep || res4.Reason != "filter_false" {
		t.Fatalf("expected drop on response_body=='-', got keep=%v reason=%q", res4.Keep, res4.Reason)
	}
}

// TestExprBackwardCompat is a regression guard: every existing data
// source row in production today uses gjson-only mapping (no `expr:`
// prefix anywhere).  This test pins that the new compiler keeps the
// legacy fast path bit-identical: same fingerprint computation, same
// MappingHits keys (raw path, not `expr:...`).
func TestExprBackwardCompat(t *testing.T) {
	prog, err := CompileKafkaProgram(KafkaFilterConfig{
		Mapping: KafkaMapping{
			Alertname:   "alertname",
			Severity:    "severity",
			Fingerprint: "alert_id",
			Labels:      map[string]string{"instance": "instance"},
		},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res, err := prog.Apply(
		[]byte(`{"alertname":"X","severity":"P3","alert_id":"abc","instance":"node-1"}`),
		"kafka", "ds-1")
	if err != nil || !res.Keep {
		t.Fatalf("apply failed: keep=%v err=%v reason=%q", res.Keep, err, res.Reason)
	}
	if res.Alert.Fingerprint != "abc" {
		t.Fatalf("Fingerprint = %q want abc", res.Alert.Fingerprint)
	}
	if res.Alert.Labels["instance"] != "node-1" {
		t.Fatalf("instance label = %q", res.Alert.Labels["instance"])
	}
	// Legacy MappingHits stays keyed by gjson path (no expr: prefix).
	if got, ok := res.MappingHits["alert_id"]; !ok || got != "abc" {
		t.Fatalf("MappingHits legacy key missing: %#v", res.MappingHits)
	}
}

// TestExprCompileError exercises the friendly Chinese error wrappers so
// the router can surface them straight to the operator.  We exercise
// three failure modes: (a) genuine syntax error in the body, (b) empty
// expr body after the `expr:` prefix, (c) syntax error in a labels cell
// (to confirm the per-cell `kafka mapping.<name> 编译失败` prefix lands).
func TestExprCompileError(t *testing.T) {
	_, err := CompileKafkaProgram(KafkaFilterConfig{
		Mapping: KafkaMapping{
			Alertname:   "alertname",
			Severity:    "severity",
			Fingerprint: `expr: strip_query(`, // unbalanced paren → syntax error
		},
	})
	if err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("expected fingerprint compile error, got %v", err)
	}
	_, err = CompileKafkaProgram(KafkaFilterConfig{
		Mapping: KafkaMapping{
			Alertname:   "alertname",
			Severity:    "severity",
			Fingerprint: `expr: `,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "expr 表达式为空") {
		t.Fatalf("expected empty-expr error, got %v", err)
	}
	_, err = CompileKafkaProgram(KafkaFilterConfig{
		Mapping: KafkaMapping{
			Alertname: "alertname",
			Severity:  "severity",
			Labels:    map[string]string{"x": `expr: 1 +`},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "labels.x") {
		t.Fatalf("expected labels.x compile error, got %v", err)
	}
}

// TestExprRuntimeError feeds a syntactically valid expr that fails at
// runtime (calling regex_replace with a bad pattern) and asserts the
// reason flips to mapping_error so dashboards can isolate this bucket.
func TestExprRuntimeError(t *testing.T) {
	prog, err := CompileKafkaProgram(KafkaFilterConfig{
		Mapping: KafkaMapping{
			Alertname:   "alertname",
			Severity:    "severity",
			Fingerprint: `expr: regex_replace(path, "(unbalanced", "x")`,
		},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res, runErr := prog.Apply(
		[]byte(`{"alertname":"X","severity":"P3","path":"/foo"}`),
		"kafka", "ds-1")
	if runErr == nil {
		t.Fatal("expected runtime error from bad regex")
	}
	if res.Reason != "mapping_error" {
		t.Fatalf("Reason = %q want mapping_error", res.Reason)
	}
}

// TestAutoFilterExprAnnotation locks down the auto-injection contract:
// every Kafka alert that passed a non-empty filter must carry the
// `kafka_filter_expr` annotation equal to the trimmed filter source so
// downstream notifications can show "why this alert was admitted".  An
// empty filter records nothing (no distinguishing condition), and a
// user-defined annotation of the same key wins (operator-overridable).
func TestAutoFilterExprAnnotation(t *testing.T) {
	payload := []byte(`{
		"alertname":"X",
		"severity":"P3",
		"response_body":"{\"ok\":true}",
		"response_code":"500",
		"path":"/foo"
	}`)

	t.Run("with_filter", func(t *testing.T) {
		prog, err := CompileKafkaProgram(KafkaFilterConfig{
			Filter: `  response_body != "-"  `, // padded → must be trimmed before injection
			Mapping: KafkaMapping{
				Alertname: "alertname",
				Severity:  "severity",
			},
		})
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		res, err := prog.Apply(payload, "kafka", "ds-1")
		if err != nil || !res.Keep {
			t.Fatalf("apply: keep=%v err=%v reason=%q", res.Keep, err, res.Reason)
		}
		got := res.Alert.Annotations["kafka_filter_expr"]
		want := `response_body != "-"`
		if got != want {
			t.Fatalf("kafka_filter_expr = %q, want %q", got, want)
		}
	})

	t.Run("empty_filter", func(t *testing.T) {
		prog, err := CompileKafkaProgram(KafkaFilterConfig{
			Mapping: KafkaMapping{
				Alertname: "alertname",
				Severity:  "severity",
			},
		})
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		res, err := prog.Apply(payload, "kafka", "ds-1")
		if err != nil || !res.Keep {
			t.Fatalf("apply: keep=%v err=%v reason=%q", res.Keep, err, res.Reason)
		}
		if v, ok := res.Alert.Annotations["kafka_filter_expr"]; ok {
			t.Fatalf("expected no kafka_filter_expr annotation, got %q", v)
		}
	})

	t.Run("user_override", func(t *testing.T) {
		prog, err := CompileKafkaProgram(KafkaFilterConfig{
			Filter: `response_code == "500"`,
			Mapping: KafkaMapping{
				Alertname: "alertname",
				Severity:  "severity",
				Annotations: map[string]string{
					"kafka_filter_expr": `expr: "custom"`,
				},
			},
		})
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		res, err := prog.Apply(payload, "kafka", "ds-1")
		if err != nil || !res.Keep {
			t.Fatalf("apply: keep=%v err=%v reason=%q", res.Keep, err, res.Reason)
		}
		if got := res.Alert.Annotations["kafka_filter_expr"]; got != "custom" {
			t.Fatalf("kafka_filter_expr = %q, want %q (user override should win)", got, "custom")
		}
	})
}
