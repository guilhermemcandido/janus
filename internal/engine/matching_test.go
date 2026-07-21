package engine

import (
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func newOrder(id uint64, side types.Side, typ types.OrderType, price int64, qty uint64) *types.Order {
	return types.NewOrder(id, "TEST", side, typ, price, qty)
}

func mustSubmit(t *testing.T, ob *OrderBook, order *types.Order) []types.Trade {
	t.Helper()
	trades, err := ob.Submit(order)
	if err != nil {
		t.Fatalf("Submit(%d) returned unexpected error: %v", order.ID, err)
	}
	return trades
}

func TestSubmit_DoesNotOverwriteCallerSetRemaining(t *testing.T) {
	ob := NewOrderBook("TEST")
	order := &types.Order{ID: 1, Symbol: "TEST", Side: types.Sell, Type: types.Limit, Price: 100, Quantity: 100, Remaining: 30}

	mustSubmit(t, ob, order)

	if order.Remaining != 30 {
		t.Fatalf("Remaining = %d, want untouched at 30 (Submit must not reset it from Quantity)", order.Remaining)
	}
}

func TestSubmit_RejectsDuplicateOrderID(t *testing.T) {
	ob := NewOrderBook("TEST")
	mustSubmit(t, ob, newOrder(1, types.Sell, types.Limit, 100, 10))

	_, err := ob.Submit(newOrder(1, types.Sell, types.Limit, 200, 5))

	if err != ErrDuplicateOrderID {
		t.Fatalf("err = %v, want ErrDuplicateOrderID", err)
	}
	resting, _ := ob.Order(1)
	if resting.Price != 100 {
		t.Fatalf("original order 1 should be untouched by the rejected duplicate, got price %d", resting.Price)
	}
}

func TestSubmit_RejectsDuplicateOrderIDEvenAfterFullyFilled(t *testing.T) {
	ob := NewOrderBook("TEST")
	mustSubmit(t, ob, newOrder(1, types.Sell, types.Limit, 100, 10))
	mustSubmit(t, ob, newOrder(2, types.Buy, types.Limit, 100, 10))

	_, err := ob.Submit(newOrder(1, types.Sell, types.Limit, 200, 5))

	if err != ErrDuplicateOrderID {
		t.Fatalf("err = %v, want ErrDuplicateOrderID even though order 1 already fully filled", err)
	}
}

func TestSubmit_RejectsZeroQuantity(t *testing.T) {
	ob := NewOrderBook("TEST")

	_, err := ob.Submit(newOrder(1, types.Buy, types.Limit, 100, 0))

	if err != ErrInvalidQuantity {
		t.Fatalf("err = %v, want ErrInvalidQuantity", err)
	}
}

func TestSubmit_RejectsSymbolMismatch(t *testing.T) {
	ob := NewOrderBook("TEST")
	order := types.NewOrder(1, "OTHER", types.Buy, types.Limit, 100, 10)

	_, err := ob.Submit(order)

	if err != ErrSymbolMismatch {
		t.Fatalf("err = %v, want ErrSymbolMismatch", err)
	}
}

func TestSubmit_ExactMatchFillsBothOrders(t *testing.T) {
	ob := NewOrderBook("TEST")
	sell := newOrder(1, types.Sell, types.Limit, 100, 50)
	buy := newOrder(2, types.Buy, types.Limit, 100, 50)

	mustSubmit(t, ob, sell)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 1 {
		t.Fatalf("got %d trades, want 1", len(trades))
	}
	tr := trades[0]
	if tr.Price != 100 || tr.Quantity != 50 || tr.MakerOrderID != 1 || tr.TakerOrderID != 2 {
		t.Fatalf("trade = %+v, want price 100 qty 50 maker 1 taker 2", tr)
	}
	if ob.BestAsk() != nil || ob.BestBid() != nil {
		t.Fatalf("expected both sides empty after exact match")
	}
	if _, ok := ob.Order(1); ok {
		t.Fatalf("fully filled maker order 1 should be removed from Orders")
	}
}

func TestSubmit_PartialFillLeavesMakerResting(t *testing.T) {
	ob := NewOrderBook("TEST")
	sell := newOrder(1, types.Sell, types.Limit, 100, 100)
	buy := newOrder(2, types.Buy, types.Limit, 100, 40)

	mustSubmit(t, ob, sell)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 1 || trades[0].Quantity != 40 {
		t.Fatalf("trades = %+v, want one trade of qty 40", trades)
	}
	if sell.Remaining != 60 {
		t.Fatalf("maker Remaining = %d, want 60", sell.Remaining)
	}
	if ob.BestAsk() == nil {
		t.Fatalf("expected maker to still be resting with quantity left")
	}
	if _, ok := ob.Order(1); !ok {
		t.Fatalf("partially filled maker should remain in Orders")
	}
}

func TestSubmit_SamePriceFIFOPriority(t *testing.T) {
	ob := NewOrderBook("TEST")
	first := newOrder(1, types.Sell, types.Limit, 100, 10)
	second := newOrder(2, types.Sell, types.Limit, 100, 10)
	buy := newOrder(3, types.Buy, types.Limit, 100, 10)

	mustSubmit(t, ob, first)
	mustSubmit(t, ob, second)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 1 || trades[0].MakerOrderID != 1 {
		t.Fatalf("trades = %+v, want single trade against order 1 (earliest arrival)", trades)
	}
	if second.Remaining != 10 {
		t.Fatalf("order 2 Remaining = %d, want untouched at 10", second.Remaining)
	}
}

func TestSubmit_SweepsMultipleMakersAtSamePriceLevel(t *testing.T) {
	ob := NewOrderBook("TEST")
	first := newOrder(1, types.Sell, types.Limit, 100, 30)
	second := newOrder(2, types.Sell, types.Limit, 100, 20)
	third := newOrder(3, types.Sell, types.Limit, 100, 50)
	buy := newOrder(4, types.Buy, types.Limit, 100, 60)

	mustSubmit(t, ob, first)
	mustSubmit(t, ob, second)
	mustSubmit(t, ob, third)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 3 {
		t.Fatalf("got %d trades, want 3", len(trades))
	}
	if trades[0].MakerOrderID != 1 || trades[0].Quantity != 30 {
		t.Fatalf("first trade = %+v, want maker 1 qty 30", trades[0])
	}
	if trades[1].MakerOrderID != 2 || trades[1].Quantity != 20 {
		t.Fatalf("second trade = %+v, want maker 2 qty 20", trades[1])
	}
	if trades[2].MakerOrderID != 3 || trades[2].Quantity != 10 {
		t.Fatalf("third trade = %+v, want maker 3 qty 10 (partial)", trades[2])
	}
	if third.Remaining != 40 {
		t.Fatalf("order 3 Remaining = %d, want 40 (partially filled, still resting)", third.Remaining)
	}
	if buy.Remaining != 0 {
		t.Fatalf("taker Remaining = %d, want 0", buy.Remaining)
	}
}

func TestSubmit_SweepsAcrossMultiplePriceLevels(t *testing.T) {
	ob := NewOrderBook("TEST")
	cheap := newOrder(1, types.Sell, types.Limit, 100, 30)
	pricier := newOrder(2, types.Sell, types.Limit, 105, 20)
	buy := newOrder(3, types.Buy, types.Limit, 110, 50)

	mustSubmit(t, ob, cheap)
	mustSubmit(t, ob, pricier)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 2 {
		t.Fatalf("got %d trades, want 2", len(trades))
	}
	if trades[0].Price != 100 || trades[0].Quantity != 30 || trades[0].MakerOrderID != 1 {
		t.Fatalf("first trade = %+v, want price 100 qty 30 maker 1", trades[0])
	}
	if trades[1].Price != 105 || trades[1].Quantity != 20 || trades[1].MakerOrderID != 2 {
		t.Fatalf("second trade = %+v, want price 105 qty 20 maker 2", trades[1])
	}
	if buy.Remaining != 0 {
		t.Fatalf("taker Remaining = %d, want 0", buy.Remaining)
	}
}

func TestSubmit_NonCrossingLimitOrderRests(t *testing.T) {
	ob := NewOrderBook("TEST")
	sell := newOrder(1, types.Sell, types.Limit, 150, 10)
	buy := newOrder(2, types.Buy, types.Limit, 100, 5)

	mustSubmit(t, ob, sell)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 0 {
		t.Fatalf("got %d trades, want 0 (prices don't cross)", len(trades))
	}
	if ob.BestBid() == nil {
		t.Fatalf("expected non-crossing buy order to rest on the bid side")
	}
	if _, ok := ob.Order(2); !ok {
		t.Fatalf("expected resting order to be tracked in Orders")
	}
}

func TestSubmit_PriceImprovementExecutesAtMakerPrice(t *testing.T) {
	ob := NewOrderBook("TEST")
	bestBid := newOrder(1, types.Buy, types.Limit, 100, 50)
	worseBid := newOrder(2, types.Buy, types.Limit, 80, 50)
	sell := newOrder(3, types.Sell, types.Limit, 90, 30)

	mustSubmit(t, ob, bestBid)
	mustSubmit(t, ob, worseBid)
	trades := mustSubmit(t, ob, sell)

	if len(trades) != 1 {
		t.Fatalf("got %d trades, want 1", len(trades))
	}
	if trades[0].Price != 100 {
		t.Fatalf("trade price = %d, want 100 (the maker's price, not the seller's 90)", trades[0].Price)
	}
	if worseBid.Remaining != 50 {
		t.Fatalf("worse bid Remaining = %d, want untouched at 50", worseBid.Remaining)
	}
}

func TestSubmit_MarketOrderFillsAcrossLevelsAndDiscardsRemainder(t *testing.T) {
	ob := NewOrderBook("TEST")
	near := newOrder(1, types.Sell, types.Limit, 100, 20)
	far := newOrder(2, types.Sell, types.Limit, 105, 10)
	buy := newOrder(3, types.Buy, types.Market, 0, 50)

	mustSubmit(t, ob, near)
	mustSubmit(t, ob, far)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 2 {
		t.Fatalf("got %d trades, want 2", len(trades))
	}
	if ob.BestAsk() != nil {
		t.Fatalf("expected both resting sells to be fully consumed")
	}
	if buy.Remaining != 20 {
		t.Fatalf("taker Remaining = %d, want 20 unfilled", buy.Remaining)
	}
	if _, ok := ob.Order(3); ok {
		t.Fatalf("market order should never rest, even partially filled")
	}
}

func TestSubmit_MarketOrderSellSideFillsAgainstBids(t *testing.T) {
	ob := NewOrderBook("TEST")
	near := newOrder(1, types.Buy, types.Limit, 100, 20)
	far := newOrder(2, types.Buy, types.Limit, 95, 10)
	sell := newOrder(3, types.Sell, types.Market, 0, 50)

	mustSubmit(t, ob, near)
	mustSubmit(t, ob, far)
	trades := mustSubmit(t, ob, sell)

	if len(trades) != 2 {
		t.Fatalf("got %d trades, want 2", len(trades))
	}
	if trades[0].Price != 100 || trades[0].MakerOrderID != 1 {
		t.Fatalf("first trade = %+v, want price 100 maker 1 (best bid first)", trades[0])
	}
	if trades[1].Price != 95 || trades[1].MakerOrderID != 2 {
		t.Fatalf("second trade = %+v, want price 95 maker 2", trades[1])
	}
	if ob.BestBid() != nil {
		t.Fatalf("expected both resting bids to be fully consumed")
	}
	if sell.Remaining != 20 {
		t.Fatalf("taker Remaining = %d, want 20 unfilled", sell.Remaining)
	}
}

func TestSubmit_MarketOrderAgainstEmptyBookProducesNoTrades(t *testing.T) {
	ob := NewOrderBook("TEST")
	buy := newOrder(1, types.Buy, types.Market, 0, 50)

	trades := mustSubmit(t, ob, buy)

	if len(trades) != 0 {
		t.Fatalf("got %d trades, want 0", len(trades))
	}
	if buy.Remaining != 50 {
		t.Fatalf("Remaining = %d, want untouched at 50", buy.Remaining)
	}
	if _, ok := ob.Order(1); ok {
		t.Fatalf("market order should never rest")
	}
}
