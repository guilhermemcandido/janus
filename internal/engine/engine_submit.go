package engine

import "github.com/guilhermemcandido/janus/internal/types"

type submitCommand struct {
	order *types.Order
	reply chan submitResult
}

type submitResult struct {
	trades []types.Trade
	err    error
}

func (e *Engine) Submit(o *types.Order) ([]types.Trade, error) {
	reply := make(chan submitResult, 1)
	r, err := call(e, submitCommand{order: o, reply: reply}, reply)
	if err != nil {
		return nil, err
	}
	return r.trades, r.err
}
