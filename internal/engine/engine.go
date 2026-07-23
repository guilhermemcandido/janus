package engine

import "github.com/guilhermemcandido/janus/internal/types"

// Engine serializes all access to one OrderBook through a single goroutine, reached only via its channel.
type Engine struct {
	book  *OrderBook
	inbox chan command
}

type kind uint8

const (
	submit kind = iota
	cancel
	bestBid
	bestAsk
	order
)

type command struct {
	kind    kind
	order   *types.Order
	orderID uint64
	reply   chan result
}

type result struct {
	trades []types.Trade
	order  *types.Order
	level  *PriceLevel
	found  bool
	err    error
}

// NewEngine wraps book; call Run in its own goroutine before using Submit/Cancel/etc.
func NewEngine(book *OrderBook) *Engine {
	return &Engine{book: book, inbox: make(chan command, 1024)}
}

// Run is the only goroutine ever allowed to touch the wrapped OrderBook directly.
func (e *Engine) Run() {
	for cmd := range e.inbox {
		switch cmd.kind {
		case submit:
			trades, err := e.book.Submit(cmd.order)
			cmd.reply <- result{trades: trades, err: err}
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
		}
	}
}

// Stop closes the inbox; Run returns once any already-queued commands drain.
func (e *Engine) Stop() {
	close(e.inbox)
}

func (e *Engine) Submit(o *types.Order) ([]types.Trade, error) {
	reply := make(chan result, 1)
	e.inbox <- command{kind: submit, order: o, reply: reply}
	r := <-reply
	return r.trades, r.err
}

func (e *Engine) Cancel(orderID uint64) (*types.Order, error) {
	reply := make(chan result, 1)
	e.inbox <- command{kind: cancel, orderID: orderID, reply: reply}
	r := <-reply
	return r.order, r.err
}

func (e *Engine) BestBid() *PriceLevel {
	reply := make(chan result, 1)
	e.inbox <- command{kind: bestBid, reply: reply}
	return (<-reply).level
}

func (e *Engine) BestAsk() *PriceLevel {
	reply := make(chan result, 1)
	e.inbox <- command{kind: bestAsk, reply: reply}
	return (<-reply).level
}

func (e *Engine) Order(orderID uint64) (*types.Order, bool) {
	reply := make(chan result, 1)
	e.inbox <- command{kind: order, orderID: orderID, reply: reply}
	r := <-reply
	return r.order, r.found
}

func (e *Engine) Symbol() string {
	return e.book.Symbol
}
