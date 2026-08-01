package engine

import (
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func TestOrderBook_RestingOrdersReturnsBestPriceFirstAndFIFO(t *testing.T) {
	ob := NewOrderBook("TEST")
	low := newOrder(types.Buy, types.Limit, 100, 10)
	high := newOrder(types.Buy, types.Limit, 105, 10)
	first := newOrder(types.Sell, types.Limit, 110, 5)
	second := newOrder(types.Sell, types.Limit, 110, 5)
	mustSubmit(t, ob, low)
	mustSubmit(t, ob, high)
	mustSubmit(t, ob, first)
	mustSubmit(t, ob, second)

	bids, asks, _ := ob.RestingOrders()

	if len(bids) != 2 || bids[0].ID != high.ID || bids[1].ID != low.ID {
		t.Fatalf("bids = %+v, want [%d, %d] (best price first)", bids, high.ID, low.ID)
	}
	if len(asks) != 2 || asks[0].ID != first.ID || asks[1].ID != second.ID {
		t.Fatalf("asks = %+v, want [%d, %d] (FIFO within the level)", asks, first.ID, second.ID)
	}
}

func TestOrderBook_RestoreRebuildsPriority(t *testing.T) {
	ob := NewOrderBook("TEST")
	first := newOrder(types.Sell, types.Limit, 100, 10)
	second := newOrder(types.Sell, types.Limit, 100, 10)
	mustSubmit(t, ob, first)
	mustSubmit(t, ob, second)
	bids, asks, seq := ob.RestingOrders()

	restored := NewOrderBook("TEST")
	restored.Restore(bids, asks, seq)

	buy := newOrder(types.Buy, types.Limit, 100, 10)
	trades := mustSubmit(t, restored, buy)

	if len(trades) != 1 || trades[0].MakerOrderID != first.ID {
		t.Fatalf("trades = %+v, want a single trade against order %d (earliest arrival)", trades, first.ID)
	}
	if _, ok := restored.Order(second.ID); !ok {
		t.Fatalf("expected order %d to still be resting after restore", second.ID)
	}
}

func TestOrderBook_RestoreContinuesSequenceCounter(t *testing.T) {
	ob := NewOrderBook("TEST")
	mustSubmit(t, ob, newOrder(types.Buy, types.Limit, 100, 10))
	bids, asks, seq := ob.RestingOrders()

	restored := NewOrderBook("TEST")
	restored.Restore(bids, asks, seq)

	next := newOrder(types.Sell, types.Limit, 105, 5)
	mustSubmit(t, restored, next)

	if next.ID <= seq {
		t.Fatalf("next.ID = %d, want greater than the restored sequence counter %d", next.ID, seq)
	}
}

func TestEngine_RestingOrdersAndRestoreRoundTrip(t *testing.T) {
	book := NewOrderBook("TEST")
	e := NewEngine(book)
	go e.Run()
	defer e.Stop()

	if _, err := e.Submit(newOrder(types.Buy, types.Limit, 100, 10)); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	bids, asks, seq := e.RestingOrders()

	restoredBook := NewOrderBook("TEST")
	restoredEngine := NewEngine(restoredBook)
	go restoredEngine.Run()
	defer restoredEngine.Stop()

	restoredEngine.Restore(bids, asks, seq)

	order, found := restoredEngine.Order(bids[0].ID)
	if !found || order.Remaining != 10 {
		t.Fatalf("Order(%d) = %+v, found=%v, want Remaining=10", bids[0].ID, order, found)
	}
}
