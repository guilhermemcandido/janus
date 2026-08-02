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

// cancelCommandPool recycles *cancelCommand the same way submitCommandPool does for Submit.
var cancelCommandPool = sync.Pool{
	New: func() any { return &cancelCommand{reply: make(chan cancelResult, 1)} },
}

func (e *Engine) Cancel(orderID uint64) (*types.Order, error) {
	cmd := cancelCommandPool.Get().(*cancelCommand)
	cmd.orderID = orderID
	r, err := call(e, cmd, cmd.reply)
	if err != nil {
		// See Submit: don't recycle a command the engine might still write a stale value into.
		return nil, err
	}
	cancelCommandPool.Put(cmd)
	return r.order, r.err
}
