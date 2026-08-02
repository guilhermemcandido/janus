package engine

import "github.com/guilhermemcandido/janus/internal/types"

// OrderBook holds both sides of the book for a single symbol.
type OrderBook struct {
	Symbol string
	bids   *BookSide
	asks   *BookSide
	orders map[uint64]*types.Order // currently resting orders, by ID
	seq    uint64

	stats   types.MarketStats
	history tradeRing
}

func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol: symbol,
		bids:   NewBookSide(types.Buy),
		asks:   NewBookSide(types.Sell),
		orders: make(map[uint64]*types.Order),
		stats:  types.MarketStats{Symbol: symbol},
	}
}

func (ob *OrderBook) BestBid() *PriceLevel {
	return ob.bids.Best()
}

func (ob *OrderBook) BestAsk() *PriceLevel {
	return ob.asks.Best()
}

// Order looks up a currently resting order by ID.
func (ob *OrderBook) Order(id uint64) (*types.Order, bool) {
	o, ok := ob.orders[id]
	return o, ok
}

// Depth returns an immutable snapshot of up to n price levels per side.
func (ob *OrderBook) Depth(n int) types.BookSnapshot {
	return types.BookSnapshot{
		Symbol: ob.Symbol,
		Bids:   ob.bids.Depth(n),
		Asks:   ob.asks.Depth(n),
	}
}

// Stats returns a snapshot of trading activity for this symbol since the engine started.
func (ob *OrderBook) Stats() types.MarketStats {
	return ob.stats
}

// History returns the most recent trades for this symbol, oldest first.
func (ob *OrderBook) History() []types.Trade {
	return ob.history.recent()
}

// RestingOrders returns every currently resting order, best price first and FIFO within each level, plus the sequence counter.
func (ob *OrderBook) RestingOrders() (bids, asks []types.Order, seq uint64) {
	return ob.bids.Orders(), ob.asks.Orders(), ob.seq
}

// Restore re-rests every order in bids and asks, in order, and sets the sequence counter.
func (ob *OrderBook) Restore(bids, asks []types.Order, seq uint64) {
	for i := range bids {
		ob.rest(&bids[i])
	}
	for i := range asks {
		ob.rest(&asks[i])
	}
	ob.seq = seq
}
