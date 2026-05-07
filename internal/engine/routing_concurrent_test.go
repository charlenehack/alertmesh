package engine

import (
	"sync"
	"testing"

	"github.com/kuzane/alertmesh/internal/ingestion"
)

// TestRouter_ConcurrentMatchesNoRace ensures Router.Match / LabelMatcher.Matches
// can be called from many goroutines in parallel without tripping the race
// detector.  The historical bug was the lazy `regexp.Compile` inside Matches
// — pre-compilation in SetRoutes makes Matches read-only.  Run with `-race`
// to validate the property end-to-end.
func TestRouter_ConcurrentMatchesNoRace(t *testing.T) {
	r := NewRouter()
	r.SetRoutes([]RouteDef{
		{
			ID:   "r1",
			Name: "p1-prod",
			Matchers: []LabelMatcher{
				{Name: "severity", Value: "P1", IsRegex: false},
				{Name: "env", Value: "^prod-(eu|us|ap)-[0-9]+$", IsRegex: true},
			},
		},
		{
			ID:   "r2",
			Name: "catchall",
			// empty matchers — fallback rule
		},
	})

	alerts := []ingestion.RawAlert{
		{Labels: map[string]string{"severity": "P1", "env": "prod-eu-1"}},
		{Labels: map[string]string{"severity": "P3", "env": "prod-us-2"}},
		{Labels: map[string]string{"severity": "P1", "env": "stage-eu-1"}},
		{Labels: map[string]string{"severity": "P1", "env": "prod-ap-9"}},
	}

	const goroutines = 64
	const perGoroutine = 5000

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				route := r.Match(alerts[i%len(alerts)])
				// Match must always return a route since the second
				// rule is the empty-matcher catch-all.
				if route == nil {
					t.Errorf("expected catch-all to fire, got nil")
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestRouter_BadRegexNeverMatches confirms a malformed regex matcher is
// flagged compileFailed at SetRoutes time and contributes a permanent miss
// to its rule (route falls through to the next one).  Side-effect:
// validates the warn-log path doesn't panic with nil refs.
func TestRouter_BadRegexNeverMatches(t *testing.T) {
	r := NewRouter()
	r.SetRoutes([]RouteDef{
		{
			ID:   "broken",
			Name: "broken-regex",
			Matchers: []LabelMatcher{
				{Name: "env", Value: "[unterminated", IsRegex: true},
			},
		},
		{
			ID:   "fallback",
			Name: "fallback",
			// empty matchers
		},
	})

	got := r.Match(ingestion.RawAlert{Labels: map[string]string{"env": "[unterminated"}})
	if got == nil || got.ID != "fallback" {
		t.Fatalf("expected fallback route, got %#v", got)
	}
}
