package engine

import "github.com/guilhermemcandido/janus/internal/types"

// tradeHistoryLimit bounds memory per symbol; older trades simply fall off the ring.
const tradeHistoryLimit = 200

// tradeRing is a fixed-size circular buffer of the most recent trades for one symbol.
type tradeRing struct {
	buf   [tradeHistoryLimit]types.Trade
	next  int
	count int
}

func (r *tradeRing) add(tr types.Trade) {
	r.buf[r.next] = tr
	r.next = (r.next + 1) % tradeHistoryLimit
	if r.count < tradeHistoryLimit {
		r.count++
	}
}

// recent returns up to tradeHistoryLimit trades, oldest first.
func (r *tradeRing) recent() []types.Trade {
	out := make([]types.Trade, r.count)
	start := (r.next - r.count + tradeHistoryLimit) % tradeHistoryLimit
	for i := 0; i < r.count; i++ {
		out[i] = r.buf[(start+i)%tradeHistoryLimit]
	}
	return out
}
