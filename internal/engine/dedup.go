package engine

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuzane/alertmesh/internal/ingestion"
)

const defaultDedupTTL = 5 * time.Minute

// Deduplicator tracks alert fingerprints with a TTL to discard repeated alerts.
//
// Concurrency model: every field is goroutine-safe by construction so the
// Kafka manager's per-row N-worker fan-out (see ingestion/kafka_manager.go)
// can hammer IsDuplicate from N goroutines without an external lock.
//
//   - `seen` is a sync.Map keyed by fingerprint string.
//   - `dedupEntry.count` is an atomic.Int64 so concurrent hits on the same
//     fingerprint don't lose increments.
//   - The Load → store-on-miss path uses LoadOrStore which is atomic at the
//     map level: only one of N concurrent inserters wins, the rest see
//     `loaded=true` and treat their alert as a duplicate of the winner.
//   - Expired entries are replaced via a CompareAndSwap loop so a stale
//     entry can never overwrite a fresh one written by a concurrent caller.
type Deduplicator struct {
	seen sync.Map
	ttl  time.Duration
}

type dedupEntry struct {
	expiresAt time.Time
	count     atomic.Int64
}

func NewDeduplicator() *Deduplicator {
	d := &Deduplicator{ttl: defaultDedupTTL}
	go d.cleanup()
	return d
}

// IsDuplicate returns true if the alert fingerprint was seen within the
// TTL window.  Safe for concurrent use; multiple goroutines racing on the
// same fingerprint will deterministically agree on exactly one "first
// occurrence" (the LoadOrStore winner) and have the rest report duplicate.
func (d *Deduplicator) IsDuplicate(alert ingestion.RawAlert) bool {
	now := time.Now()

	// Build the candidate insert up-front; we'll throw it away if some
	// other goroutine got there first.  Cheap allocation vs the cost of
	// a sync.Map round trip means there's no win in lazy-init.
	fresh := &dedupEntry{expiresAt: now.Add(d.ttl)}
	fresh.count.Store(1)

	for {
		actual, loaded := d.seen.LoadOrStore(alert.Fingerprint, fresh)
		if !loaded {
			// We won the insert race; this is the first time we've seen
			// this fingerprint within the TTL window.
			return false
		}
		entry := actual.(*dedupEntry)
		if now.Before(entry.expiresAt) {
			// Existing entry is still alive — count this hit and tell
			// the caller it's a duplicate.
			entry.count.Add(1)
			return true
		}
		// Existing entry expired.  Try to replace it atomically.
		// CompareAndSwap on sync.Map ensures we only overwrite the
		// exact stale entry we observed; if another goroutine got
		// there first we re-loop and re-evaluate against their entry.
		if d.seen.CompareAndSwap(alert.Fingerprint, actual, fresh) {
			return false
		}
	}
}

func (d *Deduplicator) cleanup() {
	ticker := time.NewTicker(d.ttl)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		d.seen.Range(func(key, value interface{}) bool {
			entry := value.(*dedupEntry)
			if now.After(entry.expiresAt) {
				// Unconditional Delete is safe: a concurrent
				// IsDuplicate that was about to insert a fresh
				// entry will simply lose its LoadOrStore race
				// with the next call after our Delete and write
				// the fresh entry it intended to write.
				d.seen.Delete(key)
			}
			return true
		})
	}
}
