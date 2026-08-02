package engine

import (
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func TestTradeRing_RecentReturnsAllWhenUnderCapacity(t *testing.T) {
	var r tradeRing
	for i := 0; i < 5; i++ {
		r.add(types.Trade{ID: uint64(i + 1)})
	}

	got := r.recent()
	if len(got) != 5 {
		t.Fatalf("len(recent()) = %d, want 5", len(got))
	}
	for i, tr := range got {
		if tr.ID != uint64(i+1) {
			t.Fatalf("recent()[%d].ID = %d, want %d", i, tr.ID, i+1)
		}
	}
}

func TestTradeRing_WrapsAroundKeepingOnlyMostRecent(t *testing.T) {
	var r tradeRing
	const total = tradeHistoryLimit + 50 // force wraparound past capacity

	for i := 0; i < total; i++ {
		r.add(types.Trade{ID: uint64(i + 1)})
	}

	got := r.recent()
	if len(got) != tradeHistoryLimit {
		t.Fatalf("len(recent()) = %d, want %d", len(got), tradeHistoryLimit)
	}

	wantFirstID := uint64(total - tradeHistoryLimit + 1)
	if got[0].ID != wantFirstID {
		t.Fatalf("recent()[0].ID = %d, want %d (oldest surviving trade)", got[0].ID, wantFirstID)
	}
	if got[len(got)-1].ID != uint64(total) {
		t.Fatalf("recent()[last].ID = %d, want %d (most recent trade)", got[len(got)-1].ID, total)
	}
	for i := 1; i < len(got); i++ {
		if got[i].ID != got[i-1].ID+1 {
			t.Fatalf("recent() not contiguous/ordered at index %d: %d then %d", i, got[i-1].ID, got[i].ID)
		}
	}
}
