package engine

import (
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func TestEngine_ConcurrentSubmitAndCancelStress(t *testing.T) {
	book := NewOrderBook("TEST")
	e := NewEngine(book)
	go e.Run()
	defer e.Stop()

	const goroutines = 100
	const opsPerGoroutine = 200

	var mu sync.Mutex
	var allOrders []*types.Order
	filled := make(map[uint64]uint64)
	cancelled := make(map[uint64]uint64)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)))
			for i := 0; i < opsPerGoroutine; i++ {
				if rng.IntN(3) == 0 {
					mu.Lock()
					var target *types.Order
					if len(allOrders) > 0 {
						target = allOrders[rng.IntN(len(allOrders))]
					}
					mu.Unlock()

					if target != nil {
						cancelledOrder, err := e.Cancel(target.ID)
						if err != nil {
							if err != ErrOrderNotFound {
								t.Errorf("Cancel returned unexpected error: %v", err)
							}
						} else {
							mu.Lock()
							cancelled[cancelledOrder.ID] = cancelledOrder.Remaining
							mu.Unlock()
						}
					}
					continue
				}

				order := randomOrder(rng, "TEST")
				trades, err := e.Submit(order)
				if err != nil {
					t.Errorf("Submit returned unexpected error: %v", err)
					continue
				}

				mu.Lock()
				allOrders = append(allOrders, order)
				for _, tr := range trades {
					filled[tr.MakerOrderID] += tr.Quantity
					filled[tr.TakerOrderID] += tr.Quantity
				}
				mu.Unlock()
			}
		}(g)
	}
	wg.Wait()

	for _, order := range allOrders {
		want := order.Quantity - filled[order.ID]
		got, wasCancelled := cancelled[order.ID]
		if !wasCancelled {
			got = currentRemaining(e.Order, order)
		}
		if got != want {
			t.Fatalf("order %d: Remaining = %d, want %d (Quantity %d minus %d filled)", order.ID, got, want, order.Quantity, filled[order.ID])
		}
	}
}

func TestExchange_ConcurrentMultiSymbolStress(t *testing.T) {
	ex := NewExchange()
	defer ex.Close()

	symbols := []string{"AAPL", "TSLA", "GOOG"}

	const goroutines = 60
	const opsPerGoroutine = 100

	var mu sync.Mutex
	orders := make(map[string][]*types.Order)
	filled := make(map[string]map[uint64]uint64)
	for _, s := range symbols {
		filled[s] = make(map[uint64]uint64)
	}

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)+1))
			for i := 0; i < opsPerGoroutine; i++ {
				symbol := symbols[rng.IntN(len(symbols))]
				eng := ex.GetOrCreateEngine(symbol)

				order := randomOrder(rng, symbol)
				trades, err := eng.Submit(order)
				if err != nil {
					t.Errorf("Submit returned unexpected error: %v", err)
					continue
				}

				mu.Lock()
				orders[symbol] = append(orders[symbol], order)
				for _, tr := range trades {
					filled[symbol][tr.MakerOrderID] += tr.Quantity
					filled[symbol][tr.TakerOrderID] += tr.Quantity
				}
				mu.Unlock()
			}
		}(g)
	}
	wg.Wait()

	for _, symbol := range symbols {
		eng := ex.GetOrCreateEngine(symbol)
		for _, order := range orders[symbol] {
			want := order.Quantity - filled[symbol][order.ID]
			got := currentRemaining(eng.Order, order)
			if got != want {
				t.Fatalf("[%s] order %d: Remaining = %d, want %d", symbol, order.ID, got, want)
			}
		}
	}
}
