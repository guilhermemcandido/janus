package engine

import "github.com/guilhermemcandido/janus/internal/types"

type restingOrdersCommand struct {
	reply chan restingOrdersResult
}

type restingOrdersResult struct {
	bids []types.Order
	asks []types.Order
	seq  uint64
}

func (e *Engine) RestingOrders() (bids, asks []types.Order, seq uint64) {
	reply := make(chan restingOrdersResult, 1)
	r, err := call(e, restingOrdersCommand{reply: reply}, reply)
	if err != nil {
		return nil, nil, 0
	}
	return r.bids, r.asks, r.seq
}

type restoreCommand struct {
	bids  []types.Order
	asks  []types.Order
	seq   uint64
	reply chan struct{}
}

func (e *Engine) Restore(bids, asks []types.Order, seq uint64) {
	reply := make(chan struct{}, 1)
	_, _ = call(e, restoreCommand{bids: bids, asks: asks, seq: seq, reply: reply}, reply)
}
