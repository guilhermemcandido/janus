package spot

import (
	"context"
	"math/rand/v2"
)

// RandomWalk is a bounded random-walk bots.PriceSource, used when there is no
// external market to derive a reference price from.
type RandomWalk struct {
	price int64
	step  int64
}

// NewRandomWalk starts a walk at initial, moving by at most step ticks per call.
func NewRandomWalk(initial, step int64) *RandomWalk {
	return &RandomWalk{price: initial, step: step}
}

// Next advances the walk by one step and returns the new price, floored at 1 tick.
func (w *RandomWalk) Next() int64 {
	delta := rand.Int64N(2*w.step+1) - w.step
	w.price += delta
	if w.price < 1 {
		w.price = 1
	}
	return w.price
}

// Price implements PriceSource.
func (w *RandomWalk) Price(ctx context.Context) (int64, error) {
	return w.Next(), nil
}
