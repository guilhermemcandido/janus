package engine

import (
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func TestBookSide_BidsBestIsHighestPrice(t *testing.T) {
	bs := NewBookSide(types.Buy)
	for _, p := range []int64{100, 105, 95, 110} {
		bs.GetOrCreateLevel(p)
	}
	if got := bs.Best(); got == nil || got.Price() != 110 {
		t.Fatalf("Best() = %v, want price 110", got)
	}
}

func TestBookSide_AsksBestIsLowestPrice(t *testing.T) {
	bs := NewBookSide(types.Sell)
	for _, p := range []int64{100, 105, 95, 110} {
		bs.GetOrCreateLevel(p)
	}
	if got := bs.Best(); got == nil || got.Price() != 95 {
		t.Fatalf("Best() = %v, want price 95", got)
	}
}

func TestBookSide_GetOrCreateLevelReturnsSameInstance(t *testing.T) {
	bs := NewBookSide(types.Buy)
	a := bs.GetOrCreateLevel(100)
	b := bs.GetOrCreateLevel(100)
	if a != b {
		t.Fatalf("GetOrCreateLevel(100) returned different instances on second call")
	}
}

func TestBookSide_RemoveLevelUpdatesBest(t *testing.T) {
	bs := NewBookSide(types.Buy)
	bs.GetOrCreateLevel(100)
	bs.GetOrCreateLevel(110)

	bs.RemoveLevel(110)

	if got := bs.Best(); got == nil || got.Price() != 100 {
		t.Fatalf("Best() after removing top level = %v, want price 100", got)
	}
}

func TestBookSide_EmptyAfterRemovingAllLevels(t *testing.T) {
	bs := NewBookSide(types.Sell)
	bs.GetOrCreateLevel(100)
	bs.RemoveLevel(100)

	if !bs.IsEmpty() {
		t.Fatalf("expected side to be empty after removing its only level")
	}
	if got := bs.Best(); got != nil {
		t.Fatalf("Best() on empty side = %v, want nil", got)
	}
}

func TestBookSide_DepthOrdersBestToWorstAndCapsAtN(t *testing.T) {
	bs := NewBookSide(types.Buy)
	bs.GetOrCreateLevel(100).Add(&types.Order{ID: 1, Remaining: 10})
	bs.GetOrCreateLevel(110).Add(&types.Order{ID: 2, Remaining: 5})
	bs.GetOrCreateLevel(95).Add(&types.Order{ID: 3, Remaining: 20})

	got := bs.Depth(2)

	if len(got) != 2 {
		t.Fatalf("Depth(2) returned %d levels, want 2", len(got))
	}
	if got[0].Price != 110 || got[0].Quantity != 5 {
		t.Fatalf("Depth(2)[0] = %+v, want price 110 qty 5", got[0])
	}
	if got[1].Price != 100 || got[1].Quantity != 10 {
		t.Fatalf("Depth(2)[1] = %+v, want price 100 qty 10", got[1])
	}
}

func TestBookSide_DepthOnEmptySideReturnsEmptySlice(t *testing.T) {
	bs := NewBookSide(types.Sell)

	got := bs.Depth(5)

	if len(got) != 0 {
		t.Fatalf("Depth(5) on empty side = %v, want empty", got)
	}
}

func TestBookSide_DepthRequestingMoreThanAvailableReturnsWhatExists(t *testing.T) {
	bs := NewBookSide(types.Buy)
	bs.GetOrCreateLevel(100).Add(&types.Order{ID: 1, Remaining: 10})

	got := bs.Depth(5)

	if len(got) != 1 {
		t.Fatalf("Depth(5) with 1 level = %v, want 1 entry", got)
	}
}

func TestBookSide_DepthWithNegativeNReturnsEmptySliceInsteadOfPanicking(t *testing.T) {
	bs := NewBookSide(types.Buy)
	bs.GetOrCreateLevel(100).Add(&types.Order{ID: 1, Remaining: 10})

	got := bs.Depth(-1)

	if len(got) != 0 {
		t.Fatalf("Depth(-1) = %v, want empty", got)
	}
}

func TestBookSide_ScansOverGapsBetweenWidelySpacedLevels(t *testing.T) {
	bs := NewBookSide(types.Sell)
	bs.GetOrCreateLevel(10).Add(&types.Order{ID: 1, Remaining: 1})
	bs.GetOrCreateLevel(5000).Add(&types.Order{ID: 2, Remaining: 2})

	bs.RemoveLevel(10)

	if got := bs.Best(); got == nil || got.Price() != 5000 {
		t.Fatalf("Best() after removing the near level = %v, want price 5000", got)
	}
	if got := bs.Depth(10); len(got) != 1 || got[0].Price != 5000 {
		t.Fatalf("Depth(10) = %+v, want a single level at price 5000", got)
	}
}

func TestBookSide_RemoveNonExistentIsNoop(t *testing.T) {
	bs := NewBookSide(types.Buy)
	bs.GetOrCreateLevel(100)

	bs.RemoveLevel(999)

	if bs.IsEmpty() {
		t.Fatalf("expected existing level to remain after removing an unknown price")
	}
}
