package engine

import (
	"sort"

	"github.com/guilhermemcandido/janus/internal/types"
)

// BookSide holds one side of the book (all bids or all asks), sorted so index 0 is always the best price.
type BookSide struct {
	side   types.Side
	levels map[int64]*PriceLevel
	prices []int64
}

func NewBookSide(side types.Side) *BookSide {
	return &BookSide{
		side:   side,
		levels: make(map[int64]*PriceLevel),
	}
}

// key maps price to an ascending sort key: bids sort by -price (highest first), asks sort by price (lowest first).
func (bs *BookSide) key(price int64) int64 {
	if bs.side == types.Buy {
		return -price
	}
	return price
}

// GetOrCreateLevel returns the PriceLevel at price, inserting it into the sorted index if it's new.
func (bs *BookSide) GetOrCreateLevel(price int64) *PriceLevel {
	if pl, ok := bs.levels[price]; ok {
		return pl
	}
	pl := NewPriceLevel(price)
	bs.levels[price] = pl

	i := sort.Search(len(bs.prices), func(i int) bool {
		return bs.key(bs.prices[i]) >= bs.key(price)
	})
	bs.prices = append(bs.prices, 0)
	copy(bs.prices[i+1:], bs.prices[i:])
	bs.prices[i] = price
	return pl
}

// RemoveLevel deletes a price level from both the map and the sorted index.
func (bs *BookSide) RemoveLevel(price int64) {
	if _, ok := bs.levels[price]; !ok {
		return
	}
	delete(bs.levels, price)

	i := sort.Search(len(bs.prices), func(i int) bool {
		return bs.key(bs.prices[i]) >= bs.key(price)
	})
	bs.prices = append(bs.prices[:i], bs.prices[i+1:]...)
}

// Level returns the PriceLevel at price without creating one, and whether it exists.
func (bs *BookSide) Level(price int64) (*PriceLevel, bool) {
	pl, ok := bs.levels[price]
	return pl, ok
}

// Best returns the PriceLevel at the best price for this side, or nil if the side is empty.
func (bs *BookSide) Best() *PriceLevel {
	if len(bs.prices) == 0 {
		return nil
	}
	return bs.levels[bs.prices[0]]
}

func (bs *BookSide) IsEmpty() bool {
	return len(bs.prices) == 0
}

// Depth returns up to n price levels from best to worst, as an immutable snapshot.
func (bs *BookSide) Depth(n int) []types.PriceLevelSnapshot {
	if n < 0 {
		n = 0
	}
	if n > len(bs.prices) {
		n = len(bs.prices)
	}
	out := make([]types.PriceLevelSnapshot, n)
	for i := 0; i < n; i++ {
		level := bs.levels[bs.prices[i]]
		out[i] = types.PriceLevelSnapshot{Price: level.Price(), Quantity: level.TotalQuantity()}
	}
	return out
}
