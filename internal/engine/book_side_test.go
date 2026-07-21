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

func TestBookSide_RemoveNonExistentIsNoop(t *testing.T) {
	bs := NewBookSide(types.Buy)
	bs.GetOrCreateLevel(100)

	bs.RemoveLevel(999)

	if bs.IsEmpty() {
		t.Fatalf("expected existing level to remain after removing an unknown price")
	}
}
