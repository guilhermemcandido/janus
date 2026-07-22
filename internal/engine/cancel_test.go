package engine

import (
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func TestCancel_RemovesRestingOrder(t *testing.T) {
	ob := NewOrderBook("TEST")
	order := newOrder(types.Sell, types.Limit, 100, 50)
	mustSubmit(t, ob, order)

	cancelled, err := ob.Cancel(order.ID)

	if err != nil {
		t.Fatalf("Cancel returned unexpected error: %v", err)
	}
	if cancelled != order {
		t.Fatalf("Cancel returned a different order than the one submitted")
	}
	if _, ok := ob.Order(order.ID); ok {
		t.Fatalf("cancelled order should no longer be tracked in Orders")
	}
}

func TestCancel_RemovesEmptiedPriceLevelFromIndex(t *testing.T) {
	ob := NewOrderBook("TEST")
	order := newOrder(types.Buy, types.Limit, 100, 50)
	mustSubmit(t, ob, order)

	if _, err := ob.Cancel(order.ID); err != nil {
		t.Fatalf("Cancel returned unexpected error: %v", err)
	}

	if ob.BestBid() != nil {
		t.Fatalf("expected BestBid to be nil after cancelling the only order at that level, got a level")
	}
}

func TestCancel_LeavesOtherOrdersAtSamePriceLevelIntact(t *testing.T) {
	ob := NewOrderBook("TEST")
	first := newOrder(types.Sell, types.Limit, 100, 10)
	second := newOrder(types.Sell, types.Limit, 100, 10)
	mustSubmit(t, ob, first)
	mustSubmit(t, ob, second)

	if _, err := ob.Cancel(first.ID); err != nil {
		t.Fatalf("Cancel returned unexpected error: %v", err)
	}

	buy := newOrder(types.Buy, types.Limit, 100, 10)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 1 || trades[0].MakerOrderID != second.ID {
		t.Fatalf("trades = %+v, want single trade against order %d (the one not cancelled)", trades, second.ID)
	}
}

func TestCancel_UnknownOrderReturnsError(t *testing.T) {
	ob := NewOrderBook("TEST")

	_, err := ob.Cancel(999)

	if err != ErrOrderNotFound {
		t.Fatalf("err = %v, want ErrOrderNotFound", err)
	}
}

func TestCancel_FullyFilledOrderCannotBeCancelled(t *testing.T) {
	ob := NewOrderBook("TEST")
	sell := newOrder(types.Sell, types.Limit, 100, 50)
	buy := newOrder(types.Buy, types.Limit, 100, 50)
	mustSubmit(t, ob, sell)
	mustSubmit(t, ob, buy)

	_, err := ob.Cancel(sell.ID)

	if err != ErrOrderNotFound {
		t.Fatalf("err = %v, want ErrOrderNotFound for an order that already fully filled", err)
	}
}

func TestCancel_PartiallyFilledOrderCanStillBeCancelled(t *testing.T) {
	ob := NewOrderBook("TEST")
	sell := newOrder(types.Sell, types.Limit, 100, 100)
	buy := newOrder(types.Buy, types.Limit, 100, 40)
	mustSubmit(t, ob, sell)
	mustSubmit(t, ob, buy)

	cancelled, err := ob.Cancel(sell.ID)

	if err != nil {
		t.Fatalf("Cancel returned unexpected error: %v", err)
	}
	if cancelled.Remaining != 60 {
		t.Fatalf("cancelled order Remaining = %d, want 60 (the unfilled portion)", cancelled.Remaining)
	}
	if ob.BestAsk() != nil {
		t.Fatalf("expected the ask side to be empty after cancelling its only remaining order")
	}
}
