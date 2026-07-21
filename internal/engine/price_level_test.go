package engine

import (
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func TestPriceLevel_FIFOOrder(t *testing.T) {
	pl := NewPriceLevel(100)
	pl.Add(&types.Order{ID: 1})
	pl.Add(&types.Order{ID: 2})
	pl.Add(&types.Order{ID: 3})

	for _, want := range []uint64{1, 2, 3} {
		got := pl.PopFront()
		if got == nil || got.ID != want {
			t.Fatalf("PopFront() = %v, want order %d", got, want)
		}
	}
	if !pl.IsEmpty() {
		t.Fatalf("expected level empty after popping all orders")
	}
}

func TestPriceLevel_RemoveMiddlePreservesOrder(t *testing.T) {
	pl := NewPriceLevel(100)
	pl.Add(&types.Order{ID: 1})
	pl.Add(&types.Order{ID: 2})
	pl.Add(&types.Order{ID: 3})

	if !pl.Remove(2) {
		t.Fatalf("Remove(2) = false, want true")
	}
	if pl.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", pl.Len())
	}

	for _, want := range []uint64{1, 3} {
		got := pl.PopFront()
		if got == nil || got.ID != want {
			t.Fatalf("PopFront() = %v, want order %d", got, want)
		}
	}
}

func TestPriceLevel_RemoveNonExistent(t *testing.T) {
	pl := NewPriceLevel(100)
	pl.Add(&types.Order{ID: 1})

	if pl.Remove(999) {
		t.Fatalf("Remove(999) = true, want false for unknown order")
	}
	if pl.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", pl.Len())
	}
}

func TestPriceLevel_FrontDoesNotRemove(t *testing.T) {
	pl := NewPriceLevel(100)
	pl.Add(&types.Order{ID: 1})

	if got := pl.Front(); got == nil || got.ID != 1 {
		t.Fatalf("Front() = %v, want order 1", got)
	}
	if pl.Len() != 1 {
		t.Fatalf("Len() = %d, want 1 (Front should not remove)", pl.Len())
	}
}

func TestPriceLevel_EmptyLevel(t *testing.T) {
	pl := NewPriceLevel(100)

	if !pl.IsEmpty() {
		t.Fatalf("expected new level to be empty")
	}
	if got := pl.Front(); got != nil {
		t.Fatalf("Front() on empty level = %v, want nil", got)
	}
	if got := pl.PopFront(); got != nil {
		t.Fatalf("PopFront() on empty level = %v, want nil", got)
	}
}
