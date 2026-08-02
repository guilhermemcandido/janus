package client

// Side is which side of the book an order rests on.
type Side int

const (
	Buy Side = iota
	Sell
)

// OrderType distinguishes resting limit orders from immediate-match market orders.
type OrderType int

const (
	Limit OrderType = iota
	Market
)

// Order is a client order as returned by the exchange.
type Order struct {
	ID        uint64
	Symbol    string
	Side      Side
	Type      OrderType
	Price     int64
	Quantity  uint64
	Remaining uint64
}

// Trade is a single match between a maker and a taker order.
type Trade struct {
	ID           uint64
	Symbol       string
	Price        int64
	Quantity     uint64
	MakerOrderID uint64
	TakerOrderID uint64
}

// PriceLevel is one level of an order book: a price and its total resting quantity.
type PriceLevel struct {
	Price    int64
	Quantity uint64
}

// BookSnapshot is an immutable view of both sides of a book at one instant.
type BookSnapshot struct {
	Symbol string
	Bids   []PriceLevel
	Asks   []PriceLevel
}

// MarketSummary summarizes trading activity for one symbol since the exchange started.
type MarketSummary struct {
	Symbol      string
	Description string
	HasTraded   bool
	LastPrice   int64
	OpenPrice   int64
	High        int64
	Low         int64
	Volume      uint64
	BestBid     *PriceLevel
	BestAsk     *PriceLevel
}
