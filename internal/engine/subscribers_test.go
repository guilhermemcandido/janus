package engine

import (
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func TestSubscribers_AddAssignsUniqueIDs(t *testing.T) {
	s := newSubscribers()
	a := s.add(make(chan types.Trade, 1))
	b := s.add(make(chan types.Trade, 1))

	if a == b {
		t.Fatalf("add() returned the same ID twice: %d", a)
	}
}

func TestSubscribers_RemoveClosesChannel(t *testing.T) {
	s := newSubscribers()
	ch := make(chan types.Trade, 1)
	id := s.add(ch)

	s.remove(id)

	if _, ok := <-ch; ok {
		t.Fatalf("expected channel to be closed after remove")
	}
}

func TestSubscribers_RemoveUnknownIsNoop(t *testing.T) {
	s := newSubscribers()
	s.remove(999)
}

func TestSubscribers_BroadcastDropsForFullChannel(t *testing.T) {
	s := newSubscribers()
	ch := make(chan types.Trade, 1)
	s.add(ch)

	tr := types.Trade{ID: 1}
	s.broadcast(tr)
	s.broadcast(tr) // channel now full; this must not block

	if len(ch) != 1 {
		t.Fatalf("len(ch) = %d, want 1 (second broadcast should have been dropped)", len(ch))
	}
}
