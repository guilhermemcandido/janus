package types

// Trade is a single match between a resting (maker) and incoming (taker) order.
type Trade struct {
	ID           uint64
	Timestamp    int64
	Price        int64
	Quantity     uint64
	MakerOrderID uint64
	TakerOrderID uint64
}
