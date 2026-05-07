package router

import (
	"errors"
	"strings"
	"testing"

	"github.com/kuzane/alertmesh/internal/ingestion"
)

// TestTranslateKafkaFilterError pins each of the operator-facing
// translations the function ships.  We pump real expr compile errors
// through ingestion.CompileKafkaProgram so the test fails the moment
// expr-lang changes its error format AND we forget to update the
// switch cases — falling back to the raw message is acceptable, but
// silently dropping a translation on upgrade should be visible.
//
// `validateMapping` is the absolute minimum CompileKafkaProgram demands
// to reach the filter-compile path.
var validateMapping = ingestion.KafkaMapping{
	Alertname: "alertname",
	Severity:  "severity",
}

func TestTranslateKafkaFilterError_Nil(t *testing.T) {
	if got := translateKafkaFilterError(nil); got != "" {
		t.Fatalf("nil error must translate to empty string, got %q", got)
	}
}

func TestTranslateKafkaFilterError_JSONEnvelopePaste(t *testing.T) {
	// Operator pasted `{"filter": "..."}` into the textarea — expr
	// reads `{...}` as a map literal and AsBool() rejects it with
	// "expected bool, but got map[string]interface {}".
	_, err := ingestion.CompileKafkaProgram(ingestion.KafkaFilterConfig{
		Filter:  `{"filter": "neq(\"x\", \"y\")"}`,
		Mapping: validateMapping,
	})
	if err == nil {
		t.Fatal("expected compile error for JSON envelope, got nil")
	}
	got := translateKafkaFilterError(err)
	if !strings.Contains(got, "JSON 粘到了表达式框") {
		t.Fatalf("translation should mention JSON envelope hint, got %q", got)
	}
	if !strings.Contains(got, err.Error()) {
		t.Fatalf("translation must preserve the original error tail, got %q", got)
	}
}

func TestTranslateKafkaFilterError_MatchesKeywordCollision(t *testing.T) {
	// `matches` is a reserved infix operator in expr-lang; using it as
	// a function call yields `unexpected token Operator("matches")`.
	_, err := ingestion.CompileKafkaProgram(ingestion.KafkaFilterConfig{
		Filter:  `matches("path", "^/api/")`,
		Mapping: validateMapping,
	})
	if err == nil {
		t.Fatal("expected compile error for matches() call, got nil")
	}
	got := translateKafkaFilterError(err)
	if !strings.Contains(got, "regex_match") {
		t.Fatalf("translation should suggest regex_match, got %q", got)
	}
}

func TestTranslateKafkaFilterError_Passthrough(t *testing.T) {
	// Errors that don't match any known pattern must come through
	// verbatim so operators can still see what went wrong.
	raw := errors.New("some unexpected expr internal error")
	if got := translateKafkaFilterError(raw); got != raw.Error() {
		t.Fatalf("unmatched error must pass through, got %q", got)
	}
}
