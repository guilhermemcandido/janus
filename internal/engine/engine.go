package engine

import (
	"sync"

	"github.com/guilhermemcandido/janus/internal/types"
)

// Engine serializes all access to one OrderBook through a single goroutine, reached only via its channel.
type Engine struct {
	book        *OrderBook
	inbox       chan command
	done        chan struct{}
	stopOnce    sync.Once
	subscribers map[uint64]chan types.Trade
	nextSubID   uint64
}

type kind uint8

const (
	submit kind = iota
	cancel
	bestBid
	bestAsk
	order
	depth
	subscribe
	unsubscribe
)

type command struct {
	kind    kind
	order   *types.Order
	orderID uint64
	depthN  int
	subID   uint64
	reply   chan result
}

type subscription struct {
	id uint64
	ch chan types.Trade
}

type result struct {
	trades   []types.Trade
	order    *types.Order
	level    *PriceLevel
	found    bool
	err      error
	snapshot types.BookSnapshot
	sub      subscription
}

// NewEngine wraps book; call Run in its own goroutine before using Submit/Cancel/etc.
func NewEngine(book *OrderBook) *Engine {
	return &Engine{
		book:        book,
		inbox:       make(chan command, 1024),
		done:        make(chan struct{}),
		subscribers: make(map[uint64]chan types.Trade),
	}
}

// Run is the only goroutine ever allowed to touch the wrapped OrderBook directly.
func (e *Engine) Run() {
	for {
		select {
		case cmd := <-e.inbox:
			e.handle(cmd)
		case <-e.done:
			for _, ch := range e.subscribers {
				close(ch)
			}
			return
		}
	}
}

func (e *Engine) handle(cmd command) {
	switch cmd.kind {
	case submit:
		trades, err := e.book.Submit(cmd.order)
		cmd.reply <- result{trades: trades, err: err}
		e.broadcast(trades)
	case cancel:
		o, err := e.book.Cancel(cmd.orderID)
		cmd.reply <- result{order: o, err: err}
	case bestBid:
		cmd.reply <- result{level: e.book.BestBid()}
	case bestAsk:
		cmd.reply <- result{level: e.book.BestAsk()}
	case order:
		o, found := e.book.Order(cmd.orderID)
		cmd.reply <- result{order: o, found: found}
	case depth:
		cmd.reply <- result{snapshot: e.book.Depth(cmd.depthN)}
	case subscribe:
		e.nextSubID++
		ch := make(chan types.Trade, 32)
		e.subscribers[e.nextSubID] = ch
		cmd.reply <- result{sub: subscription{id: e.nextSubID, ch: ch}}
	case unsubscribe:
		if ch, ok := e.subscribers[cmd.subID]; ok {
			delete(e.subscribers, cmd.subID)
			close(ch)
		}
		cmd.reply <- result{}
	}
}

// broadcast sends each trade to every subscriber, dropping it for anyone too slow to keep up rather than blocking matching.
func (e *Engine) broadcast(trades []types.Trade) {
	for _, tr := range trades {
		for _, ch := range e.subscribers {
			select {
			case ch <- tr:
			default:
			}
		}
	}
}

// Stop signals Run to exit; safe to call more than once and safe to race with in-flight Submit/Cancel/etc calls.
func (e *Engine) Stop() {
	e.stopOnce.Do(func() { close(e.done) })
}

// call sends cmd and waits for its reply, returning ErrEngineStopped instead of blocking or panicking if the engine has stopped.
func (e *Engine) call(cmd command) (result, error) {
	reply := make(chan result, 1)
	cmd.reply = reply

	select {
	case e.inbox <- cmd:
	case <-e.done:
		return result{}, ErrEngineStopped
	}

	select {
	case r := <-reply:
		return r, nil
	case <-e.done:
		return result{}, ErrEngineStopped
	}
}

func (e *Engine) Submit(o *types.Order) ([]types.Trade, error) {
	r, err := e.call(command{kind: submit, order: o})
	if err != nil {
		return nil, err
	}
	return r.trades, r.err
}

func (e *Engine) Cancel(orderID uint64) (*types.Order, error) {
	r, err := e.call(command{kind: cancel, orderID: orderID})
	if err != nil {
		return nil, err
	}
	return r.order, r.err
}

func (e *Engine) BestBid() *PriceLevel {
	r, err := e.call(command{kind: bestBid})
	if err != nil {
		return nil
	}
	return r.level
}

func (e *Engine) BestAsk() *PriceLevel {
	r, err := e.call(command{kind: bestAsk})
	if err != nil {
		return nil
	}
	return r.level
}

func (e *Engine) Order(orderID uint64) (*types.Order, bool) {
	r, err := e.call(command{kind: order, orderID: orderID})
	if err != nil {
		return nil, false
	}
	return r.order, r.found
}

// Depth returns an immutable snapshot of up to n price levels per side.
func (e *Engine) Depth(n int) types.BookSnapshot {
	r, err := e.call(command{kind: depth, depthN: n})
	if err != nil {
		return types.BookSnapshot{Symbol: e.Symbol()}
	}
	return r.snapshot
}

// Subscribe returns a channel of live trades and an unsubscribe function to stop receiving and release it.
func (e *Engine) Subscribe() (<-chan types.Trade, func()) {
	r, err := e.call(command{kind: subscribe})
	if err != nil {
		ch := make(chan types.Trade)
		close(ch)
		return ch, func() {}
	}
	sub := r.sub
	return sub.ch, func() {
		e.call(command{kind: unsubscribe, subID: sub.id})
	}
}

func (e *Engine) Symbol() string {
	return e.book.Symbol
}
