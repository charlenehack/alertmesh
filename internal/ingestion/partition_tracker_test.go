package ingestion

import (
	"math/rand"
	"sort"
	"sync"
	"testing"

	kafka "github.com/segmentio/kafka-go"
)

func mk(partition int, offset int64) kafka.Message {
	return kafka.Message{Topic: "t", Partition: partition, Offset: offset}
}

func TestPartitionTracker_InOrderAck(t *testing.T) {
	tr := newPartitionTracker()
	for i := int64(10); i < 15; i++ {
		tr.Ack(mk(0, i))
	}
	got := tr.DrainCommittable()
	if len(got) != 1 || got[0].Offset != 14 {
		t.Fatalf("want single offset=14 commit, got %+v", got)
	}
	if tr.PendingDepth() != 0 {
		t.Fatalf("pending should be empty, got %d", tr.PendingDepth())
	}
}

func TestPartitionTracker_OutOfOrderHoldsCommit(t *testing.T) {
	tr := newPartitionTracker()
	// First ack at 10 sets the floor.
	tr.Ack(mk(0, 10))
	// Then 12 arrives before 11 — must NOT advance hwm past 10.
	tr.Ack(mk(0, 12))
	got := tr.DrainCommittable()
	if len(got) != 1 || got[0].Offset != 10 {
		t.Fatalf("expected hwm at 10 only, got %+v", got)
	}
	if tr.PendingDepth() != 1 {
		t.Fatalf("expected 1 pending (offset 12), got %d", tr.PendingDepth())
	}
	// Filling the gap pulls 11 and 12 in one shot.
	tr.Ack(mk(0, 11))
	got = tr.DrainCommittable()
	if len(got) != 1 || got[0].Offset != 12 {
		t.Fatalf("after gap fill expected offset=12, got %+v", got)
	}
	if tr.PendingDepth() != 0 {
		t.Fatalf("pending should drain, got %d", tr.PendingDepth())
	}
}

func TestPartitionTracker_PerPartitionIsolation(t *testing.T) {
	tr := newPartitionTracker()
	tr.Ack(mk(0, 100))
	tr.Ack(mk(1, 50))
	// Partition 0 has 102 out of order — should not block partition 1's
	// committable progress.
	tr.Ack(mk(0, 102))
	tr.Ack(mk(1, 51))

	got := tr.DrainCommittable()
	sort.Slice(got, func(i, j int) bool { return got[i].Partition < got[j].Partition })
	if len(got) != 2 {
		t.Fatalf("want 2 partitions, got %+v", got)
	}
	if got[0].Partition != 0 || got[0].Offset != 100 {
		t.Fatalf("partition 0 must stay at 100, got %+v", got[0])
	}
	if got[1].Partition != 1 || got[1].Offset != 51 {
		t.Fatalf("partition 1 must advance to 51, got %+v", got[1])
	}
}

func TestPartitionTracker_DuplicateAckIgnored(t *testing.T) {
	tr := newPartitionTracker()
	tr.Ack(mk(0, 5))
	tr.Ack(mk(0, 5)) // duplicate — must be a no-op
	tr.Ack(mk(0, 6))

	got := tr.DrainCommittable()
	if len(got) != 1 || got[0].Offset != 6 {
		t.Fatalf("dup ignored, want offset=6, got %+v", got)
	}
	// Re-acking an already-committed offset must not re-emit.
	tr.Ack(mk(0, 6))
	if leftover := tr.DrainCommittable(); len(leftover) != 0 {
		t.Fatalf("committed offsets must not re-emit, got %+v", leftover)
	}
}

func TestPartitionTracker_DrainResetsHWM(t *testing.T) {
	tr := newPartitionTracker()
	tr.Ack(mk(0, 1))
	tr.Ack(mk(0, 2))
	if got := tr.DrainCommittable(); len(got) != 1 || got[0].Offset != 2 {
		t.Fatalf("first drain offset=2, got %+v", got)
	}
	// Second drain with no new acks returns nothing.
	if got := tr.DrainCommittable(); len(got) != 0 {
		t.Fatalf("second drain should be empty, got %+v", got)
	}
	// New acks beyond committed advance again.
	tr.Ack(mk(0, 3))
	tr.Ack(mk(0, 4))
	if got := tr.DrainCommittable(); len(got) != 1 || got[0].Offset != 4 {
		t.Fatalf("advance after drain offset=4, got %+v", got)
	}
}

// TestPartitionTracker_RandomOrderConcurrent shuffles a contiguous
// offset range, fans the acks across N goroutines, and asserts that
// drains accumulate to exactly the highest offset with no gaps left in
// pending.  Also exercises locking under load — `go test -race` will
// flag any unsynchronised access.
func TestPartitionTracker_RandomOrderConcurrent(t *testing.T) {
	const N = 1000
	const goroutines = 16

	tr := newPartitionTracker()
	offsets := make([]int64, N)
	for i := 0; i < N; i++ {
		offsets[i] = int64(i + 1)
	}
	rand.Shuffle(N, func(i, j int) { offsets[i], offsets[j] = offsets[j], offsets[i] })

	chunks := make([][]int64, goroutines)
	for i, off := range offsets {
		chunks[i%goroutines] = append(chunks[i%goroutines], off)
	}

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for _, off := range chunks[g] {
				tr.Ack(mk(0, off))
			}
		}(g)
	}
	wg.Wait()

	got := tr.DrainCommittable()
	if len(got) != 1 || got[0].Offset != int64(N) {
		t.Fatalf("after random concurrent ack want hwm=%d, got %+v", N, got)
	}
	if tr.PendingDepth() != 0 {
		t.Fatalf("pending must be empty, got %d", tr.PendingDepth())
	}
}
