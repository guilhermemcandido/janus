package engine

import (
	"math/rand/v2"
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
