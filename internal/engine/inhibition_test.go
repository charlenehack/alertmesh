package engine

import (
	"strconv"
	"sync"
	"testing"

	"github.com/kuzane/alertmesh/internal/ingestion"
)

// TestInhibitor_ZeroRulesNoLockHotPath validates the documented
// optimisation: with no rules configured Track() returns without ever
// touching sourcesMu.  We can't directly observe "lock not taken", but
// we can verify behaviour: thousands of concurrent Track calls finish
// without populating the sources map (since there are no rules to
// match) and IsInhibited returns false.  Run with `-race` to catch
// any unsynchronised access reintroduced by future edits.
func TestInhibitor_ZeroRulesNoLockHotPath(t *testing.T) {
	i := NewInhibitor()

	const goroutines = 32
	const perGoroutine = 10000

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for k := 0; k < perGoroutine; k++ {
				i.Track(ingestion.RawAlert{
					Fingerprint: strconv.Itoa(g*perGoroutine + k),
					Labels:      map[string]string{"severity": "critical"},
				})
			}
		}(g)
	}
	wg.Wait()

	if got := i.IsInhibited(AlertGroup{Labels: map[string]string{"severity": "warning"}}); got {
		t.Fatalf("with zero rules IsInhibited must be false")
	}
	if len(i.sources) != 0 {
		t.Fatalf("with zero rules no source should be tracked, got %d", len(i.sources))
	}
}

// TestInhibitor_RuleHotReloadConcurrentSafe stresses the SetRules ↔
// Track ↔ IsInhibited concurrency contract.  The atomic.Pointer
// refactor requires that callers never observe a torn slice; race
// detector will flag any regression.
func TestInhibitor_RuleHotReloadConcurrentSafe(t *testing.T) {
	i := NewInhibitor()

	stop := make(chan struct{})

	// Reloader goroutine: flips between zero rules and one rule so
	// Track / IsInhibited callers see both regimes.
	var reloaderWG sync.WaitGroup
	reloaderWG.Add(1)
	go func() {
		defer reloaderWG.Done()
		for k := 0; ; k++ {
			select {
			case <-stop:
				return
			default:
			}
			if k%2 == 0 {
				i.SetRules(nil)
			} else {
				i.SetRules([]InhibitRule{{
					Name:           "critical-blocks-warning",
					SourceMatchers: []LabelMatcher{{Name: "severity", Value: "critical"}},
					TargetMatchers: []LabelMatcher{{Name: "severity", Value: "warning"}},
					Equal:          []string{"cluster"},
				}})
			}
		}
	}()

	// Bounded workers: Track + IsInhibited under racing rule changes.
	var workerWG sync.WaitGroup
	for g := 0; g < 16; g++ {
		workerWG.Add(1)
		go func(g int) {
			defer workerWG.Done()
			for k := 0; k < 5000; k++ {
				i.Track(ingestion.RawAlert{
					Fingerprint: strconv.Itoa(g*5000 + k),
					Labels: map[string]string{
						"severity": "critical",
						"cluster":  "c1",
					},
				})
				_ = i.IsInhibited(AlertGroup{Labels: map[string]string{
					"severity": "warning",
					"cluster":  "c1",
				}})
			}
		}(g)
	}

	workerWG.Wait()
	close(stop)
	reloaderWG.Wait()
}

// TestInhibitor_TrackThenIsInhibited exercises the with-rule path end
// to end: a critical alert lands as a source, a warning group with
// matching equal-labels lookup is then inhibited.  Guards the simple
// optimisation didn't drop a behavioural branch.
func TestInhibitor_TrackThenIsInhibited(t *testing.T) {
	i := NewInhibitor()
	i.SetRules([]InhibitRule{{
		Name:           "critical-blocks-warning",
		SourceMatchers: []LabelMatcher{{Name: "severity", Value: "critical"}},
		TargetMatchers: []LabelMatcher{{Name: "severity", Value: "warning"}},
		Equal:          []string{"cluster"},
	}})

	i.Track(ingestion.RawAlert{
		Fingerprint: "abc",
		Labels:      map[string]string{"severity": "critical", "cluster": "c1"},
	})

	if !i.IsInhibited(AlertGroup{Labels: map[string]string{
		"severity": "warning",
		"cluster":  "c1",
	}}) {
		t.Fatalf("warning in c1 should be inhibited by active critical")
	}
	if i.IsInhibited(AlertGroup{Labels: map[string]string{
		"severity": "warning",
		"cluster":  "c2",
	}}) {
		t.Fatalf("warning in c2 should NOT be inhibited (cluster differs)")
	}
}
