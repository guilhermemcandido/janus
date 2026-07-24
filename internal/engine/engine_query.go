package engine

import "github.com/guilhermemcandido/janus/internal/types"

type bestBidCommand struct {
	reply chan *PriceLevel
}

func (e *Engine) BestBid() *PriceLevel {
	reply := make(chan *PriceLevel, 1)
	r, err := call(e, bestBidCommand{reply: reply}, reply)
	if err != nil {
		return nil
	}
	return r
}

type bestAskCommand struct {
	reply chan *PriceLevel
}

func (e *Engine) BestAsk() *PriceLevel {
	reply := make(chan *PriceLevel, 1)
	r, err := call(e, bestAskCommand{reply: reply}, reply)
	if err != nil {
		return nil
	}
	return r
}

type orderCommand struct {
	orderID uint64
	reply   chan orderResult
}

type orderResult struct {
	order *types.Order
	found bool
}

func (e *Engine) Order(orderID uint64) (*types.Order, bool) {
	reply := make(chan orderResult, 1)
	r, err := call(e, orderCommand{orderID: orderID, reply: reply}, reply)
	if err != nil {
		return nil, false
	}
	return r.order, r.found
}

type depthCommand struct {
	n     int
	reply chan types.BookSnapshot
}

// Depth returns an immutable snapshot of up to n price levels per side.
func (e *Engine) Depth(n int) types.BookSnapshot {
	reply := make(chan types.BookSnapshot, 1)
	r, err := call(e, depthCommand{n: n, reply: reply}, reply)
	if err != nil {
		return types.BookSnapshot{Symbol: e.Symbol()}
	}
	return r
}
