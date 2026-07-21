package types

// Side is which side of the book an order rests on.
type Side uint8

const (
	Buy Side = iota
	Sell
)

// OrderType distinguishes resting limit orders from immediate-match market orders.
type OrderType uint8

const (
	Limit OrderType = iota
	Market
)

// Order is a single client order tracked by the engine.
type Order struct {
	ID        uint64
	Side      Side
	Type      OrderType
	Price     int64
	Quantity  uint64
	Remaining uint64
	Timestamp int64
}
