package engine

// maxTickPrice bounds how large the tick array can grow, so a single wildly out-of-range price
// can't force a huge allocation - the same role a price collar plays on a real exchange.
const maxTickPrice = 1 << 20 // ~1,048,576 ticks, ~8 MiB of pointers at the high end

// tickArray is a dense array of *PriceLevel indexed directly by price, giving O(1) level lookup and
// insert in place of a binary search into a sorted slice. It only ever grows toward maxTickPrice,
// since price is always positive, and never shrinks back down.
type tickArray struct {
	levels []*PriceLevel // levels[price], valid for price in [1, len(levels))
}

func (t *tickArray) get(price int64) *PriceLevel {
	if price <= 0 || int(price) >= len(t.levels) {
		return nil
	}
	return t.levels[price]
}

// set stores pl at price, growing the backing array if needed. It reports false without storing
// anything if price falls outside (0, maxTickPrice].
func (t *tickArray) set(price int64, pl *PriceLevel) bool {
	if price <= 0 || price > maxTickPrice {
		return false
	}
	if int(price) >= len(t.levels) {
		grown := make([]*PriceLevel, price+1)
		copy(grown, t.levels)
		t.levels = grown
	}
	t.levels[price] = pl
	return true
}

func (t *tickArray) delete(price int64) {
	if price > 0 && int(price) < len(t.levels) {
		t.levels[price] = nil
	}
}
