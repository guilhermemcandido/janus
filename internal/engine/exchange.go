package engine

import (
	"math/rand/v2"
	"sync"
)

// Exchange holds one Engine per registered symbol, plus the description each was listed with.
// RWMutex over Mutex: Lookup is the hot path (every request), Register only happens at startup.
type Exchange struct {
	mu           sync.RWMutex
	engines      map[string]*Engine
	descriptions map[string]string

	// Epoch is picked once per process so clients can detect that the exchange restarted and lost all state.
	Epoch uint64
}

func NewExchange() *Exchange {
	return &Exchange{
		engines:      make(map[string]*Engine),
		descriptions: make(map[string]string),
		Epoch:        rand.Uint64(),
	}
}

// Register lists symbol on the exchange, starting its Engine goroutine the first time it's called.
// Calling it again for an already-registered symbol is a no-op; the first description wins.
func (e *Exchange) Register(symbol, description string) *Engine {
	e.mu.Lock()
	defer e.mu.Unlock()

	if eng, ok := e.engines[symbol]; ok {
		return eng
	}
	eng := NewEngine(NewOrderBook(symbol))
	go eng.Run()
	e.engines[symbol] = eng
	e.descriptions[symbol] = description
	return eng
}

// Lookup returns the Engine for a symbol that has already been registered.
func (e *Exchange) Lookup(symbol string) (*Engine, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	eng, ok := e.engines[symbol]
	return eng, ok
}

// Description returns the description symbol was registered with, or "" if it isn't registered.
func (e *Exchange) Description(symbol string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.descriptions[symbol]
}

// Symbols returns every registered symbol.
func (e *Exchange) Symbols() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	symbols := make([]string, 0, len(e.engines))
	for s := range e.engines {
		symbols = append(symbols, s)
	}
	return symbols
}

// Close stops every managed Engine's goroutine.
func (e *Exchange) Close() {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, eng := range e.engines {
		eng.Stop()
	}
}
