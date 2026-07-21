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
	Symbol    string
	Side      Side
	Type      OrderType
	Price     int64
	Quantity  uint64
	Remaining uint64
	Timestamp int64
}

// NewOrder builds an order ready to submit, with Remaining seeded from Quantity.
func NewOrder(id uint64, symbol string, side Side, typ OrderType, price int64, quantity uint64) *Order {
	return &Order{
		ID:        id,
		Symbol:    symbol,
		Side:      side,
		Type:      typ,
		Price:     price,
		Quantity:  quantity,
		Remaining: quantity,
	}
}
