package engine

import "github.com/guilhermemcandido/janus/internal/types"

type cancelCommand struct {
	orderID uint64
	reply   chan cancelResult
}

type cancelResult struct {
	order *types.Order
	err   error
}

func (e *Engine) Cancel(orderID uint64) (*types.Order, error) {
	reply := make(chan cancelResult, 1)
	r, err := call(e, cancelCommand{orderID: orderID, reply: reply}, reply)
	if err != nil {
		return nil, err
	}
	return r.order, r.err
}
