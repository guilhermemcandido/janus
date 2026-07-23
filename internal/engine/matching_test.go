package engine

import (
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func newOrder(side types.Side, typ types.OrderType, price int64, qty uint64) *types.Order {
	return types.NewOrder("TEST", side, typ, price, qty)
}

func mustSubmit(t *testing.T, ob *OrderBook, order *types.Order) []types.Trade {
	t.Helper()
	trades, err := ob.Submit(order)
	if err != nil {
		t.Fatalf("Submit returned unexpected error: %v", err)
	}
	return trades
}

func TestSubmit_DoesNotOverwriteCallerSetRemaining(t *testing.T) {
	ob := NewOrderBook("TEST")
	order := &types.Order{Symbol: "TEST", Side: types.Sell, Type: types.Limit, Price: 100, Quantity: 100, Remaining: 30}

	mustSubmit(t, ob, order)

	if order.Remaining != 30 {
		t.Fatalf("Remaining = %d, want untouched at 30 (Submit must not reset it from Quantity)", order.Remaining)
	}
}

func TestSubmit_AssignsUniqueOrderIDs(t *testing.T) {
	ob := NewOrderBook("TEST")
	a := newOrder(types.Sell, types.Limit, 100, 10)
	b := newOrder(types.Sell, types.Limit, 100, 10)

	mustSubmit(t, ob, a)
	mustSubmit(t, ob, b)

	if a.ID == 0 || b.ID == 0 {
		t.Fatalf("expected engine to assign non-zero IDs, got a.ID=%d b.ID=%d", a.ID, b.ID)
	}
	if a.ID == b.ID {
		t.Fatalf("expected distinct IDs, both got %d", a.ID)
	}
}

func TestSubmit_RejectsZeroQuantity(t *testing.T) {
	ob := NewOrderBook("TEST")

	_, err := ob.Submit(newOrder(types.Buy, types.Limit, 100, 0))

	if err != ErrInvalidQuantity {
		t.Fatalf("err = %v, want ErrInvalidQuantity", err)
	}
}

func TestSubmit_RejectsNonPositivePriceForLimitOrders(t *testing.T) {
	ob := NewOrderBook("TEST")

	for _, price := range []int64{0, -5} {
		_, err := ob.Submit(newOrder(types.Sell, types.Limit, price, 10))
		if err != ErrInvalidPrice {
			t.Fatalf("price %d: err = %v, want ErrInvalidPrice", price, err)
		}
	}
}

func TestSubmit_MarketOrderIgnoresZeroPrice(t *testing.T) {
	ob := NewOrderBook("TEST")

	if _, err := ob.Submit(newOrder(types.Buy, types.Market, 0, 10)); err != nil {
		t.Fatalf("market order with price 0 returned unexpected error: %v", err)
	}
}

func TestSubmit_RejectsSymbolMismatch(t *testing.T) {
	ob := NewOrderBook("TEST")
	order := types.NewOrder("OTHER", types.Buy, types.Limit, 100, 10)

	_, err := ob.Submit(order)

	if err != ErrSymbolMismatch {
		t.Fatalf("err = %v, want ErrSymbolMismatch", err)
	}
}

func TestSubmit_ExactMatchFillsBothOrders(t *testing.T) {
	ob := NewOrderBook("TEST")
	sell := newOrder(types.Sell, types.Limit, 100, 50)
	buy := newOrder(types.Buy, types.Limit, 100, 50)

	mustSubmit(t, ob, sell)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 1 {
		t.Fatalf("got %d trades, want 1", len(trades))
	}
	tr := trades[0]
	if tr.Price != 100 || tr.Quantity != 50 || tr.MakerOrderID != sell.ID || tr.TakerOrderID != buy.ID {
		t.Fatalf("trade = %+v, want price 100 qty 50 maker %d taker %d", tr, sell.ID, buy.ID)
	}
	if ob.BestAsk() != nil || ob.BestBid() != nil {
		t.Fatalf("expected both sides empty after exact match")
	}
	if _, ok := ob.Order(sell.ID); ok {
		t.Fatalf("fully filled maker order should be removed from Orders")
	}
}

func TestSubmit_PartialFillLeavesMakerResting(t *testing.T) {
	ob := NewOrderBook("TEST")
	sell := newOrder(types.Sell, types.Limit, 100, 100)
	buy := newOrder(types.Buy, types.Limit, 100, 40)

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
	if _, ok := ob.Order(sell.ID); !ok {
		t.Fatalf("partially filled maker should remain in Orders")
	}
}

func TestSubmit_SamePriceFIFOPriority(t *testing.T) {
	ob := NewOrderBook("TEST")
	first := newOrder(types.Sell, types.Limit, 100, 10)
	second := newOrder(types.Sell, types.Limit, 100, 10)
	buy := newOrder(types.Buy, types.Limit, 100, 10)

	mustSubmit(t, ob, first)
	mustSubmit(t, ob, second)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 1 || trades[0].MakerOrderID != first.ID {
		t.Fatalf("trades = %+v, want single trade against order %d (earliest arrival)", trades, first.ID)
	}
	if second.Remaining != 10 {
		t.Fatalf("order 2 Remaining = %d, want untouched at 10", second.Remaining)
	}
}

func TestSubmit_SweepsMultipleMakersAtSamePriceLevel(t *testing.T) {
	ob := NewOrderBook("TEST")
	first := newOrder(types.Sell, types.Limit, 100, 30)
	second := newOrder(types.Sell, types.Limit, 100, 20)
	third := newOrder(types.Sell, types.Limit, 100, 50)
	buy := newOrder(types.Buy, types.Limit, 100, 60)

	mustSubmit(t, ob, first)
	mustSubmit(t, ob, second)
	mustSubmit(t, ob, third)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 3 {
		t.Fatalf("got %d trades, want 3", len(trades))
	}
	if trades[0].MakerOrderID != first.ID || trades[0].Quantity != 30 {
		t.Fatalf("first trade = %+v, want maker %d qty 30", trades[0], first.ID)
	}
	if trades[1].MakerOrderID != second.ID || trades[1].Quantity != 20 {
		t.Fatalf("second trade = %+v, want maker %d qty 20", trades[1], second.ID)
	}
	if trades[2].MakerOrderID != third.ID || trades[2].Quantity != 10 {
		t.Fatalf("third trade = %+v, want maker %d qty 10 (partial)", trades[2], third.ID)
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
	cheap := newOrder(types.Sell, types.Limit, 100, 30)
	pricier := newOrder(types.Sell, types.Limit, 105, 20)
	buy := newOrder(types.Buy, types.Limit, 110, 50)

	mustSubmit(t, ob, cheap)
	mustSubmit(t, ob, pricier)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 2 {
		t.Fatalf("got %d trades, want 2", len(trades))
	}
	if trades[0].Price != 100 || trades[0].Quantity != 30 || trades[0].MakerOrderID != cheap.ID {
		t.Fatalf("first trade = %+v, want price 100 qty 30 maker %d", trades[0], cheap.ID)
	}
	if trades[1].Price != 105 || trades[1].Quantity != 20 || trades[1].MakerOrderID != pricier.ID {
		t.Fatalf("second trade = %+v, want price 105 qty 20 maker %d", trades[1], pricier.ID)
	}
	if buy.Remaining != 0 {
		t.Fatalf("taker Remaining = %d, want 0", buy.Remaining)
	}
}

func TestSubmit_NonCrossingLimitOrderRests(t *testing.T) {
	ob := NewOrderBook("TEST")
	sell := newOrder(types.Sell, types.Limit, 150, 10)
	buy := newOrder(types.Buy, types.Limit, 100, 5)

	mustSubmit(t, ob, sell)
	trades := mustSubmit(t, ob, buy)

	if len(trades) != 0 {
		t.Fatalf("got %d trades, want 0 (prices don't cross)", len(trades))
	}
	if ob.BestBid() == nil {
		t.Fatalf("expected non-crossing buy order to rest on the bid side")
	}
	if _, ok := ob.Order(buy.ID); !ok {
		t.Fatalf("expected resting order to be tracked in Orders")
	}
}

func TestSubmit_PriceImprovementExecutesAtMakerPrice(t *testing.T) {
	ob := NewOrderBook("TEST")
	bestBid := newOrder(types.Buy, types.Limit, 100, 50)
	worseBid := newOrder(types.Buy, types.Limit, 80, 50)
	sell := newOrder(types.Sell, types.Limit, 90, 30)

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
	near := newOrder(types.Sell, types.Limit, 100, 20)
	far := newOrder(types.Sell, types.Limit, 105, 10)
	buy := newOrder(types.Buy, types.Market, 0, 50)

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
	if _, ok := ob.Order(buy.ID); ok {
		t.Fatalf("market order should never rest, even partially filled")
	}
}

func TestSubmit_MarketOrderSellSideFillsAgainstBids(t *testing.T) {
	ob := NewOrderBook("TEST")
	near := newOrder(types.Buy, types.Limit, 100, 20)
	far := newOrder(types.Buy, types.Limit, 95, 10)
	sell := newOrder(types.Sell, types.Market, 0, 50)

	mustSubmit(t, ob, near)
	mustSubmit(t, ob, far)
	trades := mustSubmit(t, ob, sell)

	if len(trades) != 2 {
		t.Fatalf("got %d trades, want 2", len(trades))
	}
	if trades[0].Price != 100 || trades[0].MakerOrderID != near.ID {
		t.Fatalf("first trade = %+v, want price 100 maker %d (best bid first)", trades[0], near.ID)
	}
	if trades[1].Price != 95 || trades[1].MakerOrderID != far.ID {
		t.Fatalf("second trade = %+v, want price 95 maker %d", trades[1], far.ID)
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
	buy := newOrder(types.Buy, types.Market, 0, 50)

	trades := mustSubmit(t, ob, buy)

	if len(trades) != 0 {
		t.Fatalf("got %d trades, want 0", len(trades))
	}
	if buy.Remaining != 50 {
		t.Fatalf("Remaining = %d, want untouched at 50", buy.Remaining)
	}
	if _, ok := ob.Order(buy.ID); ok {
		t.Fatalf("market order should never rest")
	}
}
