package engine

import (
	"math/rand/v2"
	"sync"
	"testing"
	"time"

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

func TestEngine_BestBid(t *testing.T) {
	book := NewOrderBook("TEST")
	e := NewEngine(book)
	go e.Run()
	defer e.Stop()

	if e.BestBid() != nil {
		t.Fatalf("expected BestBid to be nil on an empty book")
	}

	buy := newOrder(types.Buy, types.Limit, 100, 10)
	if _, err := e.Submit(buy); err != nil {
		t.Fatalf("Submit returned unexpected error: %v", err)
	}

	best := e.BestBid()
	if best == nil || best.Price != 100 {
		t.Fatalf("BestBid() = %v, want price 100", best)
	}

	if _, err := e.Cancel(buy.ID); err != nil {
		t.Fatalf("Cancel returned unexpected error: %v", err)
	}
	if e.BestBid() != nil {
		t.Fatalf("expected BestBid to be nil after cancelling the only bid")
	}
}

func TestEngine_Depth(t *testing.T) {
	book := NewOrderBook("TEST")
	e := NewEngine(book)
	go e.Run()
	defer e.Stop()

	if _, err := e.Submit(newOrder(types.Buy, types.Limit, 100, 10)); err != nil {
		t.Fatalf("Submit returned unexpected error: %v", err)
	}
	if _, err := e.Submit(newOrder(types.Sell, types.Limit, 105, 5)); err != nil {
		t.Fatalf("Submit returned unexpected error: %v", err)
	}

	snap := e.Depth(10)

	if len(snap.Bids) != 1 || snap.Bids[0].Price != 100 || snap.Bids[0].Quantity != 10 {
		t.Fatalf("Bids = %+v, want [{100 10}]", snap.Bids)
	}
	if len(snap.Asks) != 1 || snap.Asks[0].Price != 105 || snap.Asks[0].Quantity != 5 {
		t.Fatalf("Asks = %+v, want [{105 5}]", snap.Asks)
	}
}

func TestEngine_SubscribeReceivesTrades(t *testing.T) {
	book := NewOrderBook("TEST")
	e := NewEngine(book)
	go e.Run()
	defer e.Stop()

	trades, unsub := e.Subscribe()
	defer unsub()

	sell := newOrder(types.Sell, types.Limit, 100, 10)
	if _, err := e.Submit(sell); err != nil {
		t.Fatalf("Submit returned unexpected error: %v", err)
	}
	buy := newOrder(types.Buy, types.Limit, 100, 10)
	if _, err := e.Submit(buy); err != nil {
		t.Fatalf("Submit returned unexpected error: %v", err)
	}

	select {
	case tr := <-trades:
		if tr.Quantity != 10 || tr.MakerOrderID != sell.ID || tr.TakerOrderID != buy.ID {
			t.Fatalf("trade = %+v, want qty 10 maker %d taker %d", tr, sell.ID, buy.ID)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected to receive a trade on the subscription channel")
	}
}

func TestEngine_UnsubscribeClosesChannel(t *testing.T) {
	book := NewOrderBook("TEST")
	e := NewEngine(book)
	go e.Run()
	defer e.Stop()

	trades, unsub := e.Subscribe()
	unsub()

	select {
	case _, ok := <-trades:
		if ok {
			t.Fatalf("expected channel to be closed after unsubscribe, got a value")
		}
	case <-time.After(time.Second):
		t.Fatalf("expected channel to be closed promptly after unsubscribe")
	}
}

func TestEngine_SubscribeSlowConsumerDoesNotBlockMatching(t *testing.T) {
	book := NewOrderBook("TEST")
	e := NewEngine(book)
	go e.Run()
	defer e.Stop()

	_, unsub := e.Subscribe()
	defer unsub()
	// deliberately never read from the subscription channel

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			if _, err := e.Submit(newOrder(types.Sell, types.Limit, 100, 1)); err != nil {
				t.Errorf("Submit returned unexpected error: %v", err)
				return
			}
			if _, err := e.Submit(newOrder(types.Buy, types.Limit, 100, 1)); err != nil {
				t.Errorf("Submit returned unexpected error: %v", err)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Submit calls appear blocked by a slow subscriber")
	}
}

func TestEngine_StopIsIdempotent(t *testing.T) {
	book := NewOrderBook("TEST")
	e := NewEngine(book)
	go e.Run()

	e.Stop()
	e.Stop()
}

func TestEngine_SubmitAfterStopReturnsError(t *testing.T) {
	book := NewOrderBook("TEST")
	e := NewEngine(book)

	runDone := make(chan struct{})
	go func() {
		e.Run()
		close(runDone)
	}()

	e.Stop()
	<-runDone

	_, err := e.Submit(newOrder(types.Buy, types.Limit, 100, 10))
	if err != ErrEngineStopped {
		t.Fatalf("err = %v, want ErrEngineStopped", err)
	}
}

func TestEngine_StopIsSafeDuringConcurrentSubmits(t *testing.T) {
	book := NewOrderBook("TEST")
	e := NewEngine(book)
	go e.Run()

	const goroutines = 50
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)))
			for i := 0; i < 100; i++ {
				_, err := e.Submit(randomOrder(rng, "TEST"))
				if err != nil && err != ErrEngineStopped {
					t.Errorf("Submit returned unexpected error: %v", err)
					return
				}
			}
		}(g)
	}

	e.Stop()
	wg.Wait()
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
		got := currentRemaining(e.Order, order)
		if got != want {
			t.Fatalf("order %d: Remaining = %d, want %d (Quantity %d minus %d filled)", order.ID, got, want, order.Quantity, filled[order.ID])
		}
	}
}
