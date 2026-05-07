package ingestion

// partitionTracker is the per-Reader bookkeeping that turns the
// out-of-order ack stream produced by the M-processor pool back into a
// strictly monotonic, per-partition contiguous offset stream that
// segmentio/kafka-go's CommitMessages requires for at-least-once
// semantics.
//
// Why we need it: with the new fetcher / processors / committer
// pipeline, processor A may finish offset 100 before processor B
// finishes offset 99 of the same partition.  Committing 100 before 99
// is acked would advance the consumer-group offset past unprocessed
// data — a crash between the commit and offset-99's processing would
// silently drop that record.  Tracker ensures we only ever expose to
// the committer the highest offset whose entire prefix has been acked.
//
// Design choices:
//   - Per-partition ack buffer is map[int64]kafka.Message rather than a
//     heap.  Hot path is "ack offset == hwm+1", which is O(1) hash lookup;
//     out-of-order acks are bounded by (channel_capacity + processor_count),
//     so the buffer stays small and a map outperforms a heap on real
//     workloads.
//   - We keep the kafka.Message itself (not just the offset) because
//     kafka-go's CommitMessages takes a slice of messages — it pulls
//     topic/partition/offset off each one.  Re-creating a synthetic
//     kafka.Message in the committer would duplicate kafka-go-internal
//     fields and risk drift across releases.

import (
	"sync"

	kafka "github.com/segmentio/kafka-go"
)

type partitionTracker struct {
	mu sync.Mutex
	// pending[partition] holds acks that arrived before their
	// predecessor — they wait here until the gap is filled.  Cleared
	// the moment the contiguous prefix swallows them.
	pending map[int]map[int64]kafka.Message
	// hwm[partition] is the highest offset whose entire prefix is
	// acked but not yet committed.  DrainCommittable returns and then
	// resets these to the just-emitted value (kafka-go remembers the
	// commit broker-side; we don't need to keep history locally).
	hwm map[int]kafka.Message
	// committedHWM[partition] is the offset the committer has already
	// shipped to the broker.  Used to detect contiguous progression
	// after a fresh start where hwm is still zero-valued.
	committedHWM map[int]int64
	// firstSeen[partition] tracks whether we've ever observed any
	// ack for this partition.  The very first ack defines the floor
	// — we cannot assume offset 0 is the next-expected because the
	// consumer group may have committed somewhere far past 0 already.
	firstSeen map[int]bool
}

func newPartitionTracker() *partitionTracker {
	return &partitionTracker{
		pending:      map[int]map[int64]kafka.Message{},
		hwm:          map[int]kafka.Message{},
		committedHWM: map[int]int64{},
		firstSeen:    map[int]bool{},
	}
}

// Ack records that processing finished for msg.  Out-of-order acks are
// stashed; a contiguous run starting at the next-expected offset
// advances hwm to the highest contiguous offset.  Safe for concurrent
// use from N processor goroutines.
func (t *partitionTracker) Ack(msg kafka.Message) {
	t.mu.Lock()
	defer t.mu.Unlock()

	p := msg.Partition

	if !t.firstSeen[p] {
		// First ack for this partition: it defines the contiguous
		// floor.  Anything below it would be a duplicate the
		// consumer-group already passed; anything equal-or-above
		// will be tracked from here.
		t.firstSeen[p] = true
		t.committedHWM[p] = msg.Offset - 1
	}

	// Already covered by a previous commit (e.g. broker rebalance
	// retransmit) — drop silently; committing again is harmless but
	// pollutes pending with duplicates.
	if msg.Offset <= t.committedHWM[p] {
		return
	}
	if cur, ok := t.hwm[p]; ok && msg.Offset <= cur.Offset {
		return
	}

	expected := t.committedHWM[p] + 1
	if cur, ok := t.hwm[p]; ok {
		expected = cur.Offset + 1
	}

	if msg.Offset != expected {
		// Out of order — buffer until the gap fills.
		bucket := t.pending[p]
		if bucket == nil {
			bucket = map[int64]kafka.Message{}
			t.pending[p] = bucket
		}
		bucket[msg.Offset] = msg
		return
	}

	// Contiguous: advance hwm and consume any pending follow-ons.
	t.hwm[p] = msg
	bucket := t.pending[p]
	if bucket == nil {
		return
	}
	for {
		next := t.hwm[p].Offset + 1
		nextMsg, ok := bucket[next]
		if !ok {
			break
		}
		delete(bucket, next)
		t.hwm[p] = nextMsg
	}
	if len(bucket) == 0 {
		delete(t.pending, p)
	}
}

// DrainCommittable returns one kafka.Message per partition whose
// contiguous prefix has advanced since the last drain.  The committer
// hands the slice straight to reader.CommitMessages.  After the call
// the tracker treats those offsets as committed and won't return them
// again.
func (t *partitionTracker) DrainCommittable() []kafka.Message {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.hwm) == 0 {
		return nil
	}
	out := make([]kafka.Message, 0, len(t.hwm))
	for p, msg := range t.hwm {
		out = append(out, msg)
		t.committedHWM[p] = msg.Offset
		delete(t.hwm, p)
	}
	return out
}

// PendingDepth reports how many out-of-order acks are buffered across
// all partitions.  Used by the committer's shutdown path to decide
// whether to keep ticking until the pipeline drains.
func (t *partitionTracker) PendingDepth() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, bucket := range t.pending {
		n += len(bucket)
	}
	return n
}
