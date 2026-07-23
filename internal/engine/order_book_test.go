package engine

import (
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func TestOrderBook_DepthCombinesBothSides(t *testing.T) {
	ob := NewOrderBook("TEST")
	mustSubmit(t, ob, newOrder(types.Buy, types.Limit, 100, 10))
	mustSubmit(t, ob, newOrder(types.Buy, types.Limit, 95, 20))
	mustSubmit(t, ob, newOrder(types.Sell, types.Limit, 105, 5))

	snap := ob.Depth(10)

	if snap.Symbol != "TEST" {
		t.Fatalf("Symbol = %q, want TEST", snap.Symbol)
	}
	if len(snap.Bids) != 2 || snap.Bids[0].Price != 100 || snap.Bids[1].Price != 95 {
		t.Fatalf("Bids = %+v, want [100 95]", snap.Bids)
	}
	if len(snap.Asks) != 1 || snap.Asks[0].Price != 105 {
		t.Fatalf("Asks = %+v, want [105]", snap.Asks)
	}
}
