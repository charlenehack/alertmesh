package engine

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuzane/alertmesh/internal/ingestion"
)

// TestAggregator_ShardedConcurrentAdd hammers Add from many goroutines
// with disjoint group_keys (different alertname per goroutine) and
// asserts every group eventually fires its callback exactly once.
//
// Run with `-race` to validate that sharded mutex bookkeeping doesn't
// regress.  The historical bug class this guards against: the timer's
// AfterFunc callback running on a different shard's mutex than Add
// took, leaving an orphaned entry in the map.
func TestAggregator_ShardedConcurrentAdd(t *testing.T) {
	a := NewAggregator()
	a.groupWait = 10 * time.Millisecond

	const goroutines = 64
	const groupsPerGoroutine = 50

	var fired atomic.Int64
	cb := func(g AlertGroup) {
		fired.Add(1)
		if len(g.Alerts) == 0 {
			t.Errorf("callback fired for empty group %s", g.GroupKey)
		}
	}

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for k := 0; k < groupsPerGoroutine; k++ {
				alert := ingestion.RawAlert{
					Fingerprint: strconv.Itoa(g*groupsPerGoroutine + k),
					Labels: map[string]string{
						"alertname": "alert-" + strconv.Itoa(g) + "-" + strconv.Itoa(k),
						"severity":  "warning",
					},
				}
				a.Add(alert, nil, cb)
			}
		}(g)
	}
	wg.Wait()

	// Allow timers to fire (groupWait + jitter for a busy host with race detector).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fired.Load() == int64(goroutines*groupsPerGoroutine) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if got, want := fired.Load(), int64(goroutines*groupsPerGoroutine); got != want {
		t.Fatalf("expected %d callbacks, got %d", want, got)
	}

	// All shards' maps must drain back to empty after timers fire.
	for i := range a.shards {
		a.shards[i].mu.Lock()
		n := len(a.shards[i].groups)
		a.shards[i].mu.Unlock()
		if n != 0 {
			t.Errorf("shard %d leaked %d groups", i, n)
		}
	}
}

// TestAggregator_SameKeyAggregates ensures repeated Adds for the same
// group_key still fold into one group + one callback (sharding only
// changes the lock layout, not the semantics).
func TestAggregator_SameKeyAggregates(t *testing.T) {
	a := NewAggregator()
	a.groupWait = 25 * time.Millisecond

	const adds = 500
	var fired atomic.Int64
	var alertCount atomic.Int64
	cb := func(g AlertGroup) {
		fired.Add(1)
		alertCount.Add(int64(len(g.Alerts)))
	}

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < adds; k++ {
				a.Add(ingestion.RawAlert{
					Fingerprint: "fp",
					Labels: map[string]string{
						"alertname": "same",
						"severity":  "warning",
					},
				}, nil, cb)
			}
		}()
	}
	wg.Wait()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fired.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if fired.Load() != 1 {
		t.Fatalf("same group_key must fire exactly once, got %d", fired.Load())
	}
	if alertCount.Load() != int64(16*adds) {
		t.Fatalf("expected %d alerts in the single group, got %d", 16*adds, alertCount.Load())
	}
}

// TestAggregator_ShardDistributionEven sanity-checks the shardFor
// distribution: 1024 random keys must land in every shard so we can
// trust the sharding actually relieves contention rather than piling
// onto one shard.
func TestAggregator_ShardDistributionEven(t *testing.T) {
	a := NewAggregator()
	hits := make(map[*aggregatorShard]int)
	for i := 0; i < 1024; i++ {
		key := computeGroupKey(map[string]string{"alertname": "k" + strconv.Itoa(i)}, []string{"alertname"})
		hits[a.shardFor(key)]++
	}
	if len(hits) != aggregatorShards {
		t.Fatalf("expected all %d shards used, got %d", aggregatorShards, len(hits))
	}
	// Allow ±50% spread from the mean (1024/16 = 64) — generous,
	// just guards against a one-shard pileup.
	mean := 1024 / aggregatorShards
	for s, n := range hits {
		if n < mean/2 || n > mean*2 {
			t.Errorf("shard %p got %d hits, mean %d (suspicious distribution)", s, n, mean)
		}
	}
}
