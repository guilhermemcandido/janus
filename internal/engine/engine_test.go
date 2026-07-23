package engine

import (
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/guilhermemcandido/janus/internal/types"
)

func TestEngine_SubmitCancelAndQueries(t *testing.T) {
	book := NewOrderBook("TEST")
	e := NewEngine(book)
	go e.Run()
	defer e.Stop()

	sell := newOrder(types.Sell, types.Limit, 100, 50)
	if _, err := e.Submit(sell); err != nil {
		t.Fatalf("Submit returned unexpected error: %v", err)
	}

	buy := newOrder(types.Buy, types.Limit, 100, 20)
	trades, err := e.Submit(buy)
	if err != nil {
		t.Fatalf("Submit returned unexpected error: %v", err)
	}
	if len(trades) != 1 || trades[0].Quantity != 20 {
		t.Fatalf("trades = %+v, want one trade of qty 20", trades)
	}

	if e.BestAsk() == nil {
		t.Fatalf("expected the partially filled sell to still be resting")
	}

	cancelled, err := e.Cancel(sell.ID)
	if err != nil {
		t.Fatalf("Cancel returned unexpected error: %v", err)
	}
	if cancelled.Remaining != 30 {
		t.Fatalf("cancelled Remaining = %d, want 30", cancelled.Remaining)
	}

	if e.BestAsk() != nil {
		t.Fatalf("expected ask side empty after cancelling its only order")
	}
	if _, found := e.Order(sell.ID); found {
		t.Fatalf("expected order to no longer be found after cancel")
	}
}

func TestEngine_ConcurrentSubmitsAreRaceFree(t *testing.T) {
	book := NewOrderBook("TEST")
	e := NewEngine(book)
	go e.Run()
	defer e.Stop()

	const goroutines = 50
	const perGoroutine = 40

	var wg sync.WaitGroup
	var mu sync.Mutex
	var allOrders []*types.Order
	filled := make(map[uint64]uint64)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)))
			for i := 0; i < perGoroutine; i++ {
				order := randomOrder(rng, "TEST")
				trades, err := e.Submit(order)
				if err != nil {
					t.Errorf("Submit returned unexpected error: %v", err)
					return
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
		if order.Remaining != want {
			t.Fatalf("order %d: Remaining = %d, want %d (Quantity %d minus %d filled)", order.ID, order.Remaining, want, order.Quantity, filled[order.ID])
		}
	}
}
