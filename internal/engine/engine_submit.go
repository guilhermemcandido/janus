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

// submitCommandPool recycles *submitCommand, reply channel included: passing a value type through
// Engine's chan any would box a fresh heap copy on every call, but a pointer boxes for free.
var submitCommandPool = sync.Pool{
	New: func() any { return &submitCommand{reply: make(chan submitResult, 1)} },
}

func (e *Engine) Submit(o *types.Order) ([]types.Trade, error) {
	cmd := submitCommandPool.Get().(*submitCommand)
	cmd.order = o
	r, err := call(e, cmd, cmd.reply)
	if err != nil {
		// The engine may still deliver into reply despite this done-triggered return - don't
		// recycle a command something could still write a stale value into.
		return nil, err
	}
	submitCommandPool.Put(cmd)
	return r.trades, r.err
}
