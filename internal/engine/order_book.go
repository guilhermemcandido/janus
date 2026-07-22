package engine

import "github.com/guilhermemcandido/janus/internal/types"

// OrderBook holds both sides of the book for a single symbol.
type OrderBook struct {
	Symbol string
	bids   *BookSide
	asks   *BookSide
	orders map[uint64]*types.Order // currently resting orders, by ID
	seq    uint64
}

func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol: symbol,
		bids:   NewBookSide(types.Buy),
		asks:   NewBookSide(types.Sell),
		orders: make(map[uint64]*types.Order),
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
