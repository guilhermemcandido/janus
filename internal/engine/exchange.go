package engine

import (
	"math/rand/v2"
	"sync"
)

// Exchange holds one Engine per traded symbol, creating each (and its OrderBook and goroutine) on first use.
type Exchange struct {
	mu      sync.Mutex
	engines map[string]*Engine

	// Epoch is picked once per process so clients can detect that the exchange restarted and lost all state.
	Epoch uint64
}

func NewExchange() *Exchange {
	return &Exchange{engines: make(map[string]*Engine), Epoch: rand.Uint64()}
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

// Symbols returns every symbol with an active Engine.
func (e *Exchange) Symbols() []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	symbols := make([]string, 0, len(e.engines))
	for s := range e.engines {
		symbols = append(symbols, s)
	}
	return symbols
}

// Close stops every managed Engine's goroutine.
func (e *Exchange) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, eng := range e.engines {
		eng.Stop()
	}
}
