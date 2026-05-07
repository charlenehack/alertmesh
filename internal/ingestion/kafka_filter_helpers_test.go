package ingestion

import (
	"strings"
	"testing"
)

// helperMapping is the minimum-viable mapping for filter-helper tests:
// alertname / severity are required by CompileKafkaProgram, everything
// else is left default so the tests focus exclusively on filter
// behaviour rather than mapping side-effects.
var helperMapping = KafkaMapping{
	Alertname: "alertname",
	Severity:  "severity",
}

func compileFilter(t *testing.T, filter string) *KafkaProgram {
	t.Helper()
	prog, err := CompileKafkaProgram(KafkaFilterConfig{
		Filter:  filter,
		Mapping: helperMapping,
	})
	if err != nil {
		t.Fatalf("CompileKafkaProgram(%q): %v", filter, err)
	}
	return prog
}

func mustApply(t *testing.T, prog *KafkaProgram, payload string) KafkaApplyResult {
	t.Helper()
	res, err := prog.ApplyForConsumer([]byte(payload), "kafka", "ds-helpers")
	if err != nil {
		t.Fatalf("Apply(%s): %v", payload, err)
	}
	return res
}

// TestKafkaFilterHelpers_NeqMissingFieldReturnsFalse pins the whole
// reason this helper exists: under raw expr, `level != "DEBUG"` on a
// payload that lacks `level` evaluates to true (nil != "DEBUG"), so the
// filter silently passes every message.  `neq("level", "DEBUG")` must
// return false in that situation, dropping the message the way the
// legacy DSL would have.
func TestKafkaFilterHelpers_NeqMissingFieldReturnsFalse(t *testing.T) {
	prog := compileFilter(t, `neq("level", "DEBUG")`)

	// Payload lacks `level` entirely — must be dropped.
	res := mustApply(t, prog, `{"alertname":"X","severity":"P3"}`)
	if res.Keep {
		t.Fatalf("missing level should DROP the message, got Keep=true (Reason=%q)", res.Reason)
	}
	if res.Reason != "filter_false" {
		t.Fatalf("expected Reason=filter_false on drop, got %q", res.Reason)
	}

	// Sanity: when level is present and matches, also drop.
	res = mustApply(t, prog, `{"alertname":"X","severity":"P3","level":"DEBUG"}`)
	if res.Keep {
		t.Fatalf("level==DEBUG should DROP, got Keep=true")
	}

	// And when level is present and differs, keep.
	res = mustApply(t, prog, `{"alertname":"X","severity":"P3","level":"ERROR"}`)
	if !res.Keep {
		t.Fatalf("level==ERROR should KEEP, got drop (Reason=%q)", res.Reason)
	}
}

// TestKafkaFilterHelpers_AllStringHelpers walks every string-shaped
// helper through both a hit case and a miss case so a regression in any
// one of them surfaces as a single failing subtest rather than dragging
// the whole pipeline down.
func TestKafkaFilterHelpers_AllStringHelpers(t *testing.T) {
	type tc struct {
		name    string
		filter  string
		payload string
		keep    bool
	}
	cases := []tc{
		// has
		{"has hit", `has("level")`, `{"alertname":"X","severity":"P3","level":"INFO"}`, true},
		{"has miss", `has("level")`, `{"alertname":"X","severity":"P3"}`, false},

		// eq
		{"eq hit", `eq("level", "ERROR")`, `{"alertname":"X","severity":"P3","level":"ERROR"}`, true},
		{"eq value miss", `eq("level", "ERROR")`, `{"alertname":"X","severity":"P3","level":"INFO"}`, false},
		{"eq field miss", `eq("level", "ERROR")`, `{"alertname":"X","severity":"P3"}`, false},

		// oneof — variadic, also exercises "in but expr keyword conflict"
		{"oneof hit", `oneof("severity", "P0", "P1")`, `{"alertname":"X","severity":"P0"}`, true},
		{"oneof miss value", `oneof("severity", "P0", "P1")`, `{"alertname":"X","severity":"P3"}`, false},
		{"oneof field miss", `oneof("severity", "P0", "P1")`, `{"alertname":"X"}`, false},

		// regex_match — uses regex cache shared with regex_replace.
		// Named regex_match (not matches) because `matches` is a
		// reserved infix operator in expr-lang.
		{"regex_match hit", `regex_match("path", "^/api/v1/")`, `{"alertname":"X","severity":"P3","path":"/api/v1/users"}`, true},
		{"regex_match non-match", `regex_match("path", "^/api/v1/")`, `{"alertname":"X","severity":"P3","path":"/healthz"}`, false},
		{"regex_match field miss", `regex_match("path", "^/api/v1/")`, `{"alertname":"X","severity":"P3"}`, false},

		// not_empty — placeholder rules: "", "-", "null" all count as empty
		{"not_empty real value", `not_empty("trace_id")`, `{"alertname":"X","severity":"P3","trace_id":"abc"}`, true},
		{"not_empty empty string", `not_empty("trace_id")`, `{"alertname":"X","severity":"P3","trace_id":""}`, false},
		{"not_empty dash placeholder", `not_empty("trace_id")`, `{"alertname":"X","severity":"P3","trace_id":"-"}`, false},
		{"not_empty null placeholder", `not_empty("trace_id")`, `{"alertname":"X","severity":"P3","trace_id":"null"}`, false},
		{"not_empty field miss", `not_empty("trace_id")`, `{"alertname":"X","severity":"P3"}`, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog := compileFilter(t, c.filter)
			res := mustApply(t, prog, c.payload)
			if res.Keep != c.keep {
				t.Fatalf("filter=%q payload=%q got Keep=%v Reason=%q want Keep=%v",
					c.filter, c.payload, res.Keep, res.Reason, c.keep)
			}
		})
	}
}

// TestKafkaFilterHelpers_NumericComparisons covers the gt/gte/lt/lte
// family across the three payload shapes the helpers must accept:
// native JSON number, string-encoded number (real-world Higress access
// logs ship status_code as the string "500"), and missing field.
func TestKafkaFilterHelpers_NumericComparisons(t *testing.T) {
	type tc struct {
		name    string
		filter  string
		payload string
		keep    bool
	}
	cases := []tc{
		{"gte int hit", `gte("status_code", 500)`, `{"alertname":"X","severity":"P3","status_code":500}`, true},
		{"gte int miss", `gte("status_code", 500)`, `{"alertname":"X","severity":"P3","status_code":499}`, false},
		{"gte string-number hit", `gte("status_code", 500)`, `{"alertname":"X","severity":"P3","status_code":"500"}`, true},
		{"gte string-number miss", `gte("status_code", 500)`, `{"alertname":"X","severity":"P3","status_code":"200"}`, false},
		{"gte field missing", `gte("status_code", 500)`, `{"alertname":"X","severity":"P3"}`, false},
		{"gte non-numeric value", `gte("status_code", 500)`, `{"alertname":"X","severity":"P3","status_code":"abc"}`, false},

		{"gt float threshold", `gt("ratio", 0.5)`, `{"alertname":"X","severity":"P3","ratio":0.6}`, true},
		{"lt strict bound", `lt("status_code", 500)`, `{"alertname":"X","severity":"P3","status_code":500}`, false},
		{"lte inclusive bound", `lte("status_code", 500)`, `{"alertname":"X","severity":"P3","status_code":500}`, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog := compileFilter(t, c.filter)
			res := mustApply(t, prog, c.payload)
			if res.Keep != c.keep {
				t.Fatalf("filter=%q payload=%q got Keep=%v Reason=%q want Keep=%v",
					c.filter, c.payload, res.Keep, res.Reason, c.keep)
			}
		})
	}
}

// TestKafkaFilterHelpers_ComplexComposite checks a realistic multi-helper
// expression of the sort the user said they actually run: include only
// when severity is P0/P1, the namespace isn't kube-system, and the path
// is under /api/.  Five payloads cover the cartesian product so a
// short-circuit bug in &&  shows up as a failure on a specific row.
func TestKafkaFilterHelpers_ComplexComposite(t *testing.T) {
	const filter = `oneof("severity", "P0", "P1") && neq("namespace", "kube-system") && regex_match("path", "^/api/")`
	prog := compileFilter(t, filter)

	type row struct {
		name    string
		payload string
		keep    bool
		why     string
	}
	rows := []row{
		{
			name:    "all conditions met",
			payload: `{"alertname":"X","severity":"P0","namespace":"prod","path":"/api/v1/users"}`,
			keep:    true,
		},
		{
			name:    "severity excluded",
			payload: `{"alertname":"X","severity":"P3","namespace":"prod","path":"/api/v1"}`,
			keep:    false,
			why:     "severity not in P0/P1",
		},
		{
			name:    "namespace excluded by neq",
			payload: `{"alertname":"X","severity":"P0","namespace":"kube-system","path":"/api/v1"}`,
			keep:    false,
			why:     "neq drops kube-system",
		},
		{
			name:    "path doesn't match prefix",
			payload: `{"alertname":"X","severity":"P0","namespace":"prod","path":"/healthz"}`,
			keep:    false,
			why:     "matches returns false",
		},
		{
			name:    "namespace MISSING — neq must drop, not pass",
			payload: `{"alertname":"X","severity":"P0","path":"/api/v1"}`,
			keep:    false,
			why:     "missing namespace → neq returns false (the bug we're fixing)",
		},
	}

	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			res := mustApply(t, prog, r.payload)
			if res.Keep != r.keep {
				t.Fatalf("payload=%s got Keep=%v Reason=%q want Keep=%v (%s)",
					r.payload, res.Keep, res.Reason, r.keep, r.why)
			}
		})
	}
}

// TestKafkaFilterHelpers_RuntimeMissingHelperDrops covers the
// known-weak side of the env-injection design: typos like
// `oneoff(...)` compile clean (they're treated as undefined identifiers
// under expr.AllowUndefinedVariables), but at runtime the call lands on
// nil, expr propagates the error, and Apply returns Reason=filter_error.
// This test pins that contract so a future env / expr upgrade that
// silently changes the behaviour to "evaluate to nil → cast to false →
// keep the message" surfaces immediately.
func TestKafkaFilterHelpers_RuntimeMissingHelperDrops(t *testing.T) {
	prog := compileFilter(t, `oneoff("severity", "P0")`)
	res, err := prog.ApplyForConsumer(
		[]byte(`{"alertname":"X","severity":"P0"}`),
		"kafka",
		"ds-typo",
	)
	// We accept either: (a) Apply returns a runtime error with the
	// matching reason, or (b) Apply returns Keep=false with the
	// matching reason.  Both signal the operator something is wrong
	// instead of silently passing every message.
	if err == nil && res.Keep {
		t.Fatalf("typo'd helper must NOT silently keep messages, got Keep=true err=nil")
	}
	if err != nil && !strings.Contains(err.Error(), "filter") {
		t.Fatalf("filter runtime error should mention 'filter', got %v", err)
	}
}
