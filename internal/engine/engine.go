package engine

import (
	"sync"

	"github.com/guilhermemcandido/janus/internal/types"
)

// Engine serializes all access to one OrderBook through a single goroutine, reached only via its channel.
type Engine struct {
	book     *OrderBook
	inbox    chan any
	done     chan struct{}
	stopOnce sync.Once
	subs     *subscribers
}

// NewEngine wraps book; call Run in its own goroutine before using Submit/Cancel/etc.
func NewEngine(book *OrderBook) *Engine {
	return &Engine{
		book:  book,
		inbox: make(chan any, 1024),
		done:  make(chan struct{}),
		subs:  newSubscribers(),
	}
}

// Run is the only goroutine ever allowed to touch the wrapped OrderBook directly.
func (e *Engine) Run() {
	for {
		select {
		case msg := <-e.inbox:
			e.handle(msg)
		case <-e.done:
			e.subs.closeAll()
			return
		}
	}
}

func (e *Engine) handle(msg any) {
	switch cmd := msg.(type) {
	case submitCommand:
		trades, err := e.book.Submit(cmd.order)
		cmd.reply <- submitResult{trades: trades, err: err}
		e.broadcast(trades)
	case cancelCommand:
		o, err := e.book.Cancel(cmd.orderID)
		cmd.reply <- cancelResult{order: o, err: err}
	case bestBidCommand:
		cmd.reply <- snapshotLevel(e.book.BestBid())
	case bestAskCommand:
		cmd.reply <- snapshotLevel(e.book.BestAsk())
	case orderCommand:
		o, found := e.book.Order(cmd.orderID)
		cmd.reply <- orderResult{order: o, found: found}
	case depthCommand:
		cmd.reply <- e.book.Depth(cmd.n)
	case statsCommand:
		cmd.reply <- e.book.Stats()
	case tradeHistoryCommand:
		cmd.reply <- e.book.History()
	case subscribeCommand:
		ch := make(chan types.Trade, 32)
		id := e.subs.add(ch)
		cmd.reply <- subscription{id: id, ch: ch}
	case unsubscribeCommand:
		e.subs.remove(cmd.subID)
		cmd.reply <- struct{}{}
	case restingOrdersCommand:
		bids, asks, seq := e.book.RestingOrders()
		cmd.reply <- restingOrdersResult{bids: bids, asks: asks, seq: seq}
	case restoreCommand:
		e.book.Restore(cmd.bids, cmd.asks, cmd.seq)
		cmd.reply <- struct{}{}
	}
}

// Stop signals Run to exit; safe to call more than once and safe to race with in-flight Submit/Cancel/etc calls.
func (e *Engine) Stop() {
	e.stopOnce.Do(func() { close(e.done) })
}

// call sends msg on the inbox and waits on reply, returning ErrEngineStopped instead of blocking or panicking if the engine has stopped.
func call[R any](e *Engine, msg any, reply chan R) (R, error) {
	select {
	case e.inbox <- msg:
	case <-e.done:
		var zero R
		return zero, ErrEngineStopped
	}

	select {
	case r := <-reply:
		return r, nil
	case <-e.done:
		var zero R
		return zero, ErrEngineStopped
	}
}

func (e *Engine) Symbol() string {
	return e.book.Symbol
}

// snapshotLevel copies pl's price and quantity while still on the engine's own goroutine, since
// pl itself keeps mutating after this call and must never be handed to another goroutine.
func snapshotLevel(pl *PriceLevel) *types.PriceLevelSnapshot {
	if pl == nil {
		return nil
	}
	return &types.PriceLevelSnapshot{Price: pl.Price(), Quantity: pl.TotalQuantity()}
}
