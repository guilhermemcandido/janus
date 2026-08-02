package engine

import (
	"fmt"
	"math/rand/v2"
	"sync/atomic"
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func BenchmarkSubmit(b *testing.B) {
	ob := NewOrderBook("BENCH")
	rng := rand.New(rand.NewPCG(1, 1))

	orders := make([]*types.Order, b.N)
	for i := range orders {
		orders[i] = randomOrder(rng, "BENCH")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob.Submit(orders[i])
	}
}

func BenchmarkEngineSubmit(b *testing.B) {
	e := NewEngine(NewOrderBook("BENCH"))
	go e.Run()
	defer e.Stop()
	rng := rand.New(rand.NewPCG(1, 1))

	orders := make([]*types.Order, b.N)
	for i := range orders {
		orders[i] = randomOrder(rng, "BENCH")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Submit(orders[i])
	}
}

// restingBuyOrders submits n non-crossing buy limit orders (an empty ask side never matches them) at
// varying prices, spreading them across price levels the same way a resting book actually would.
func restingBuyOrders(n int, submit func(*types.Order)) []*types.Order {
	rng := rand.New(rand.NewPCG(1, 1))
	orders := make([]*types.Order, n)
	for i := range orders {
		price := int64(50 + rng.IntN(200))
		qty := uint64(1 + rng.IntN(20))
		orders[i] = types.NewOrder("BENCH", types.Buy, types.Limit, price, qty)
		submit(orders[i])
	}
	return orders
}

func BenchmarkCancel(b *testing.B) {
	ob := NewOrderBook("BENCH")
	orders := restingBuyOrders(b.N, func(o *types.Order) { ob.Submit(o) })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob.Cancel(orders[i].ID)
	}
}

func BenchmarkEngineCancel(b *testing.B) {
	e := NewEngine(NewOrderBook("BENCH"))
	go e.Run()
	defer e.Stop()
	orders := restingBuyOrders(b.N, func(o *types.Order) { e.Submit(o) })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Cancel(orders[i].ID)
	}
}

// benchRandSource hands out a distinct, deterministic PCG seed per call so parallel benchmark
// goroutines each get their own *rand.Rand without racing on a shared one.
var benchRandCounter atomic.Uint64

func benchRand() *rand.Rand {
	s := benchRandCounter.Add(1)
	return rand.New(rand.NewPCG(s, s^0x9e3779b97f4a7c15))
}

// BenchmarkExchangeSubmit measures Submit under concurrent load spread across several symbols, one
// Engine goroutine each - the shape make simulate actually produces with its many bots and markets.
func BenchmarkExchangeSubmit(b *testing.B) {
	const numSymbols = 8
	ex := NewExchange()
	defer ex.Close()

	symbols := make([]string, numSymbols)
	for i := range symbols {
		symbols[i] = fmt.Sprintf("SYM%d", i)
		ex.Register(symbols[i], "")
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		rng := benchRand()
		i := 0
		for pb.Next() {
			symbol := symbols[i%numSymbols]
			i++
			eng, _ := ex.Lookup(symbol)
			eng.Submit(randomOrder(rng, symbol))
		}
	})
}
