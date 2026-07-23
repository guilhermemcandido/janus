package engine

import "sync"

// Exchange holds one Engine per traded symbol, creating each (and its OrderBook and goroutine) on first use.
type Exchange struct {
	mu      sync.Mutex
	engines map[string]*Engine
}

func NewExchange() *Exchange {
	return &Exchange{engines: make(map[string]*Engine)}
}

// GetOrCreateEngine returns the Engine for symbol, starting its goroutine the first time it's requested.
func (e *Exchange) GetOrCreateEngine(symbol string) *Engine {
	e.mu.Lock()
	defer e.mu.Unlock()

	if eng, ok := e.engines[symbol]; ok {
		return eng
	}
	eng := NewEngine(NewOrderBook(symbol))
	go eng.Run()
	e.engines[symbol] = eng
	return eng
}

// Close stops every managed Engine's goroutine.
func (e *Exchange) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, eng := range e.engines {
		eng.Stop()
	}
}
