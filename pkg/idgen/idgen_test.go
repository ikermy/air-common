package idgen

import (
	"sync"
	"testing"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
)

func TestNewRejectsInvalidNode(t *testing.T) {
	if _, err := New(maxNodeID + 1); err == nil {
		t.Fatal("New() must reject node id above maximum")
	}
}

func TestNextUniqueAndMonotonic(t *testing.T) {
	g := MustNew(7)
	const n = 100_000
	seen := make(map[uint64]struct{}, n)
	var prev uint64
	for i := 0; i < n; i++ {
		id := g.Next()
		if id == 0 {
			t.Fatal("Next() returned zero")
		}
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id %d", id)
		}
		seen[id] = struct{}{}
		if i > 0 && id <= prev {
			t.Fatalf("id is not monotonic: prev=%d current=%d", prev, id)
		}
		prev = id
	}
}

func TestNextConcurrentUnique(t *testing.T) {
	g := MustNew(3)
	const workers = 16
	const perWorker = 5_000
	var wg sync.WaitGroup
	results := make(chan uint64, workers*perWorker)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				results <- g.Next()
			}
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[uint64]struct{}, workers*perWorker)
	for id := range results {
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id %d", id)
		}
		seen[id] = struct{}{}
	}
}

func TestClockRollbackKeepsMonotonic(t *testing.T) {
	g := MustNew(1)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return base }
	first := g.Next()

	// Simulate a clock rollback by one hour.
	g.now = func() time.Time { return base.Add(-time.Hour) }
	second := g.Next()
	if second <= first {
		t.Fatalf("clock rollback broke monotonicity: first=%d second=%d", first, second)
	}
}

func TestSequenceOverflow(t *testing.T) {
	g := MustNew(1)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return base }

	seen := make(map[uint64]struct{}, maxSequence+2)
	for i := 0; i < maxSequence+2; i++ {
		id := g.Next()
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id %d at iteration %d", id, i)
		}
		seen[id] = struct{}{}
	}
}

func TestTypedIDs(t *testing.T) {
	g := MustNew(5)
	var _ comdom.UserID = g.NextUserID()
	var _ comdom.DialogID = g.NextDialogID()
	var _ comdom.ResponderID = g.NextResponderID()
	var _ comdom.MessageID = g.NextMessageID()
	var _ comdom.BotID = g.NextBotID()
	var _ comdom.ChannelID = g.NextChannelID()
}

func TestDecode(t *testing.T) {
	g := MustNew(9)
	id := g.Next()
	ts, node, seq := Decode(id)
	if node != 9 {
		t.Fatalf("node = %d, want 9", node)
	}
	if seq > maxSequence {
		t.Fatalf("sequence = %d", seq)
	}
	if ts.Before(time.Now().Add(-time.Minute)) || ts.After(time.Now().Add(time.Minute)) {
		t.Fatalf("unexpected timestamp %s", ts)
	}
}
