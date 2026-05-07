package engine

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kuzane/alertmesh/internal/ingestion"
)

// TestDeduplicator_ConcurrentSameFingerprint stresses the dedup path for the
// scenario the new Kafka manager hits when multiple workers share the same
// alert fingerprint: exactly one of them must observe `IsDuplicate=false`
// (the "first" — winner of the LoadOrStore race) and the rest must report
// duplicate.  Run under `-race` to catch the historic count++/Load-then-Store
// data races.
func TestDeduplicator_ConcurrentSameFingerprint(t *testing.T) {
	d := &Deduplicator{ttl: defaultDedupTTL}

	const goroutines = 100
	const perGoroutine = 1000

	var firsts atomic.Int64
	var dups atomic.Int64

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				if d.IsDuplicate(ingestion.RawAlert{Fingerprint: "fp-shared"}) {
					dups.Add(1)
				} else {
					firsts.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	totalCalls := int64(goroutines * perGoroutine)
	if firsts.Load()+dups.Load() != totalCalls {
		t.Fatalf("call accounting mismatch: firsts=%d dups=%d want_total=%d", firsts.Load(), dups.Load(), totalCalls)
	}
	// Across the whole run we must have exactly one first-occurrence
	// — the very first goroutine through the LoadOrStore.  Every other
	// call is a duplicate.
	if firsts.Load() != 1 {
		t.Fatalf("expected exactly 1 first-occurrence, got %d (TTL race?)", firsts.Load())
	}

	// The internal hit counter on the entry must equal the duplicate
	// count plus the very first hit (which sets count to 1).
	v, ok := d.seen.Load("fp-shared")
	if !ok {
		t.Fatal("entry missing after stress run")
	}
	entry := v.(*dedupEntry)
	wantCount := dups.Load() + 1
	if got := entry.count.Load(); got != wantCount {
		t.Fatalf("entry count race: got %d want %d", got, wantCount)
	}
}

// TestDeduplicator_ConcurrentDistinctFingerprints exercises the LoadOrStore
// insert path under fan-out from many goroutines writing distinct keys.
// We assert "no panics under -race" (data race detector is the main
// signal) plus that every distinct fingerprint is recorded exactly once.
func TestDeduplicator_ConcurrentDistinctFingerprints(t *testing.T) {
	d := &Deduplicator{ttl: defaultDedupTTL}

	const goroutines = 50
	const perGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				fp := fmt.Sprintf("fp-%d-%d", g, i)
				if d.IsDuplicate(ingestion.RawAlert{Fingerprint: fp}) {
					t.Errorf("unique fingerprint %s reported as duplicate", fp)
					return
				}
			}
		}()
	}
	wg.Wait()

	count := 0
	d.seen.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != goroutines*perGoroutine {
		t.Fatalf("seen size mismatch: got %d want %d", count, goroutines*perGoroutine)
	}
}
