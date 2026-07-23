package types

// PriceLevelSnapshot is an immutable view of one price level: its price and total resting quantity.
type PriceLevelSnapshot struct {
	Price    int64
	Quantity uint64
}

// BookSnapshot is an immutable view of both sides of a book at one instant.
type BookSnapshot struct {
	Symbol string
	Bids   []PriceLevelSnapshot
	Asks   []PriceLevelSnapshot
}
