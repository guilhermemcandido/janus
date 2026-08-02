package engine

import "github.com/guilhermemcandido/janus/internal/types"

type bestBidCommand struct {
	reply chan *types.PriceLevelSnapshot
}

// BestBid returns an immutable snapshot of the best bid, or nil if the book has none. A snapshot,
// not the live *PriceLevel, since that object keeps mutating on the engine's own goroutine after
// this call returns and callers here run on a different goroutine.
func (e *Engine) BestBid() *types.PriceLevelSnapshot {
	reply := make(chan *types.PriceLevelSnapshot, 1)
	r, err := call(e, bestBidCommand{reply: reply}, reply)
	if err != nil {
		return nil
	}
	return r
}

type bestAskCommand struct {
	reply chan *types.PriceLevelSnapshot
}

// BestAsk returns an immutable snapshot of the best ask, or nil if the book has none.
func (e *Engine) BestAsk() *types.PriceLevelSnapshot {
	reply := make(chan *types.PriceLevelSnapshot, 1)
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

type statsCommand struct {
	reply chan types.MarketStats
}

// Stats returns a snapshot of trading activity for this symbol since the engine started.
func (e *Engine) Stats() types.MarketStats {
	reply := make(chan types.MarketStats, 1)
	r, err := call(e, statsCommand{reply: reply}, reply)
	if err != nil {
		return types.MarketStats{Symbol: e.Symbol()}
	}
	return r
}

type tradeHistoryCommand struct {
	reply chan []types.Trade
}

// TradeHistory returns the most recent trades for this symbol, oldest first.
func (e *Engine) TradeHistory() []types.Trade {
	reply := make(chan []types.Trade, 1)
	r, err := call(e, tradeHistoryCommand{reply: reply}, reply)
	if err != nil {
		return nil
	}
	return r
}
