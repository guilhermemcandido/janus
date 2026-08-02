package engine

import (
	"sync"

	"github.com/guilhermemcandido/janus/internal/types"
)

type submitCommand struct {
	order *types.Order
	reply chan submitResult
}

type submitResult struct {
	trades []types.Trade
	err    error
}

// submitReplyPool recycles reply channels across Submit calls - by far the hottest command, and a
// fresh channel per call was the single largest source of engine allocations under sustained load.
var submitReplyPool = sync.Pool{
	New: func() any { return make(chan submitResult, 1) },
}

func (e *Engine) Submit(o *types.Order) ([]types.Trade, error) {
	reply := submitReplyPool.Get().(chan submitResult)
	r, err := call(e, submitCommand{order: o, reply: reply}, reply)
	if err != nil {
		// The engine may still be about to deliver into reply despite this done-triggered return
		// (Run's select doesn't have to prefer done over an already-enqueued message) - don't
		// recycle a channel something could still write a stale value into.
		return nil, err
	}
	submitReplyPool.Put(reply)
	return r.trades, r.err
}
