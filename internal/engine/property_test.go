package engine

import (
	"math/rand/v2"
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func randomOrder(rng *rand.Rand, symbol string) *types.Order {
	side := types.Buy
	if rng.IntN(2) == 1 {
		side = types.Sell
	}
	typ := types.Limit
	if rng.IntN(5) == 0 {
		typ = types.Market
	}
	price := int64(90 + rng.IntN(21))
	qty := uint64(1 + rng.IntN(20))
	return types.NewOrder(symbol, side, typ, price, qty)
}

func TestProperty_QuantityConservationAcrossRandomOrders(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	ob := NewOrderBook("TEST")

	var submitted []*types.Order
	filled := make(map[uint64]uint64)

	const n = 2000
	for i := 0; i < n; i++ {
		order := randomOrder(rng, "TEST")
		trades := mustSubmit(t, ob, order)
		submitted = append(submitted, order)

		for _, tr := range trades {
			filled[tr.MakerOrderID] += tr.Quantity
			filled[tr.TakerOrderID] += tr.Quantity
		}
	}

	for _, order := range submitted {
		want := order.Quantity - filled[order.ID]
		if order.Remaining != want {
			t.Fatalf("order %d: Remaining = %d, want %d (Quantity %d minus %d filled)", order.ID, order.Remaining, want, order.Quantity, filled[order.ID])
		}
	}
}

func TestProperty_BookNeverCrossesAfterAnyOrder(t *testing.T) {
	rng := rand.New(rand.NewPCG(2, 2))
	ob := NewOrderBook("TEST")

	const n = 2000
	for i := 0; i < n; i++ {
		mustSubmit(t, ob, randomOrder(rng, "TEST"))

		bid := ob.BestBid()
		ask := ob.BestAsk()
		if bid != nil && ask != nil && bid.Price() >= ask.Price() {
			t.Fatalf("book crossed after order %d: best bid %d >= best ask %d", i, bid.Price(), ask.Price())
		}
	}
}

func TestProperty_FIFOOrderPreservedAtSharedPrice(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 3))
	ob := NewOrderBook("TEST")

	const n = 50
	var ids []uint64
	var total uint64
	for i := 0; i < n; i++ {
		qty := uint64(1 + rng.IntN(10))
		order := newOrder(types.Sell, types.Limit, 100, qty)
		mustSubmit(t, ob, order)
		ids = append(ids, order.ID)
		total += qty
	}

	sweep := newOrder(types.Buy, types.Limit, 100, total)
	trades := mustSubmit(t, ob, sweep)

	if len(trades) != n {
		t.Fatalf("got %d trades, want %d (one per resting order)", len(trades), n)
	}
	for i, tr := range trades {
		if tr.MakerOrderID != ids[i] {
			t.Fatalf("trade %d matched maker %d, want %d (FIFO order broken)", i, tr.MakerOrderID, ids[i])
		}
	}
}
