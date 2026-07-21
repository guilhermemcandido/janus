package engine

import "github.com/guilhermemcandido/janus/internal/types"

// OrderBook holds both sides of the book for a single symbol.
type OrderBook struct {
	Symbol string
	Bids   *BookSide
	Asks   *BookSide
	Orders map[uint64]*types.Order
}

func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol: symbol,
		Bids:   NewBookSide(types.Buy),
		Asks:   NewBookSide(types.Sell),
		Orders: make(map[uint64]*types.Order),
	}
}
