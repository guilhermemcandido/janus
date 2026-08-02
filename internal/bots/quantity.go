package bots

import "math/rand/v2"

// JitterQuantity randomizes base by roughly ±50%, floored at 1, so repeated orders don't all
// land on the exact same size.
func JitterQuantity(base uint64) uint64 {
	factor := 0.5 + rand.Float64()
	qty := int64(float64(base) * factor)
	if qty < 1 {
		qty = 1
	}
	return uint64(qty)
}
