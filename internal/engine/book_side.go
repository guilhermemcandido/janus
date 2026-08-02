package engine

import "github.com/guilhermemcandido/janus/internal/types"

// maxFreeLevels bounds how many emptied PriceLevels a BookSide holds onto for reuse, so a market
// that keeps drifting to new prices forever can't grow this into an unbounded memory leak.
const maxFreeLevels = 1024

// BookSide holds one side of the book (all bids or all asks) in a tickArray, tracking the current
// best price directly so Best/GetOrCreateLevel/RemoveLevel don't need to search the common case.
type BookSide struct {
	side  types.Side
	tick  tickArray
	best  int64 // meaningful only while count > 0
	count int
	free  []*PriceLevel // emptied levels ready for reuse; safe unsynchronized since only the owning Engine's goroutine ever touches a BookSide
}

func NewBookSide(side types.Side) *BookSide {
	return &BookSide{side: side}
}

// better reports whether a is a better price than b for this side: higher for bids, lower for asks.
func (bs *BookSide) better(a, b int64) bool {
	if bs.side == types.Buy {
		return a > b
	}
	return a < b
}

// GetOrCreateLevel returns the PriceLevel at price, inserting it if it's new. Callers must already
// validate price against maxTickPrice (Submit does) - this just trusts it and grows.
func (bs *BookSide) GetOrCreateLevel(price int64) *PriceLevel {
	if pl := bs.tick.get(price); pl != nil {
		return pl
	}
	pl := bs.newPriceLevel(price)
	bs.tick.set(price, pl)
	bs.count++
	if bs.count == 1 || bs.better(price, bs.best) {
		bs.best = price
	}
	return pl
}

// newPriceLevel reuses an emptied level from the freelist when one's available, avoiding a fresh
// index map for what's typically a price level being refilled after a fill.
func (bs *BookSide) newPriceLevel(price int64) *PriceLevel {
	if n := len(bs.free); n > 0 {
		pl := bs.free[n-1]
		bs.free = bs.free[:n-1]
		pl.reset(price)
		return pl
	}
	return NewPriceLevel(price)
}

// RemoveLevel deletes a price level from the tick array, scanning toward the next occupied price
// if it was the best, and returns it to the freelist for reuse.
func (bs *BookSide) RemoveLevel(price int64) {
	pl := bs.tick.get(price)
	if pl == nil {
		return
	}
	bs.tick.delete(price)
	bs.count--

	if price == bs.best {
		if bs.count == 0 {
			bs.best = 0
		} else {
			bs.best = bs.nextOccupied(price)
		}
	}

	if len(bs.free) < maxFreeLevels {
		bs.free = append(bs.free, pl)
	}
}

// nextOccupied scans away from price, toward worse prices, for the next occupied level. Bounded by
// the tick array's range; cheap in practice since real order flow keeps levels dense near the top.
func (bs *BookSide) nextOccupied(price int64) int64 {
	step := bs.step()
	for p := price + step; p >= 1 && p <= maxTickPrice; p += step {
		if bs.tick.get(p) != nil {
			return p
		}
	}
	return 0
}

// step is the direction of decreasing price desirability: down for bids, up for asks.
func (bs *BookSide) step() int64 {
	if bs.side == types.Buy {
		return -1
	}
	return 1
}

// Level returns the PriceLevel at price without creating one, and whether it exists.
func (bs *BookSide) Level(price int64) (*PriceLevel, bool) {
	pl := bs.tick.get(price)
	return pl, pl != nil
}

// Best returns the PriceLevel at the best price for this side, or nil if the side is empty.
func (bs *BookSide) Best() *PriceLevel {
	if bs.count == 0 {
		return nil
	}
	return bs.tick.get(bs.best)
}

func (bs *BookSide) IsEmpty() bool {
	return bs.count == 0
}

// walk visits every occupied level from best to worst, stopping early if visit returns false.
func (bs *BookSide) walk(visit func(*PriceLevel) bool) {
	if bs.count == 0 {
		return
	}
	step := bs.step()
	visited := 0
	for p := bs.best; p >= 1 && p <= maxTickPrice && visited < bs.count; p += step {
		pl := bs.tick.get(p)
		if pl == nil {
			continue
		}
		visited++
		if !visit(pl) {
			return
		}
	}
}

// Orders returns every resting order on this side, best price first and FIFO within each level.
func (bs *BookSide) Orders() []types.Order {
	var out []types.Order
	bs.walk(func(pl *PriceLevel) bool {
		for _, o := range pl.Orders() {
			out = append(out, *o)
		}
		return true
	})
	return out
}

// Depth returns up to n price levels from best to worst, as an immutable snapshot.
func (bs *BookSide) Depth(n int) []types.PriceLevelSnapshot {
	if n < 0 {
		n = 0
	}
	out := make([]types.PriceLevelSnapshot, 0, min(n, bs.count))
	bs.walk(func(pl *PriceLevel) bool {
		if len(out) >= n {
			return false
		}
		out = append(out, types.PriceLevelSnapshot{Price: pl.Price(), Quantity: pl.TotalQuantity()})
		return true
	})
	return out
}
