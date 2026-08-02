package engine

import (
	"sync"

	"github.com/guilhermemcandido/janus/internal/types"
)

type cancelCommand struct {
	orderID uint64
	reply   chan cancelResult
}

type cancelResult struct {
	order *types.Order
	err   error
}

// cancelReplyPool recycles reply channels across Cancel calls, the same way submitReplyPool does
// for Submit.
var cancelReplyPool = sync.Pool{
	New: func() any { return make(chan cancelResult, 1) },
}

func (e *Engine) Cancel(orderID uint64) (*types.Order, error) {
	reply := cancelReplyPool.Get().(chan cancelResult)
	r, err := call(e, cancelCommand{orderID: orderID, reply: reply}, reply)
	if err != nil {
		// See Submit: don't recycle a channel the engine might still write a stale value into.
		return nil, err
	}
	cancelReplyPool.Put(reply)
	return r.order, r.err
}
