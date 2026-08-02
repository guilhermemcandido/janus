package persistence

import (
	"math/rand/v2"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/guilhermemcandido/janus/internal/engine"
	"github.com/guilhermemcandido/janus/internal/types"
)

func TestSave_ConcurrentCallsNeverCorruptTheFile(t *testing.T) {
	ex := engine.NewExchange()
	defer ex.Close()
	if _, err := ex.Register("AAPL", "Apple Inc.").Submit(types.NewOrder("AAPL", types.Buy, types.Limit, 100, 10)); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	path := filepath.Join(t.TempDir(), "exchange.snapshot.json")

	const concurrentSaves = 20
	var wg sync.WaitGroup
	for i := 0; i < concurrentSaves; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := Save(ex, path); err != nil {
				t.Errorf("Save returned error: %v", err)
			}
		}()
	}
	wg.Wait()

	snap, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error after concurrent saves: %v", err)
	}
	if len(snap.Books) != 1 || len(snap.Books[0].Bids) != 1 {
		t.Fatalf("Books = %+v, want one book with one resting bid", snap.Books)
	}
}

func TestSave_ConcurrentWithLiveTradingIsRaceFree(t *testing.T) {
	ex := engine.NewExchange()
	defer ex.Close()

	symbols := []string{"AAPL", "TSLA", "GOOG"}
	for _, s := range symbols {
		ex.Register(s, s)
	}

	path := filepath.Join(t.TempDir(), "exchange.snapshot.json")
	stop := make(chan struct{})
	var wg sync.WaitGroup

	const traders = 10
	for i := 0; i < traders; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)))
			var resting []uint64
			for {
				select {
				case <-stop:
					return
				default:
				}
				symbol := symbols[rng.IntN(len(symbols))]
				eng, _ := ex.Lookup(symbol) // pre-registered above, before any goroutine started
				if len(resting) > 0 && rng.IntN(3) == 0 {
					id := resting[rng.IntN(len(resting))]
					_, _ = eng.Cancel(id)
					continue
				}
				side := types.Buy
				if rng.IntN(2) == 1 {
					side = types.Sell
				}
				price := int64(90 + rng.IntN(21))
				order := types.NewOrder(symbol, side, types.Limit, price, uint64(1+rng.IntN(10)))
				if _, err := eng.Submit(order); err == nil {
					resting = append(resting, order.ID)
				}
			}
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := Save(ex, path); err != nil {
				t.Errorf("Save returned error during concurrent trading: %v", err)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	time.Sleep(1 * time.Second)
	close(stop)
	wg.Wait()

	snap, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if snap == nil {
		t.Fatalf("expected a snapshot to exist after concurrent saving")
	}

	restored := engine.NewExchange()
	defer restored.Close()
	Restore(restored, snap)

	for _, symbol := range symbols {
		eng, ok := restored.Lookup(symbol)
		if !ok {
			t.Fatalf("[%s] not registered after Restore", symbol)
		}
		bid := eng.BestBid()
		ask := eng.BestAsk()
		if bid != nil && ask != nil && bid.Price >= ask.Price {
			t.Fatalf("[%s] restored book crossed: best bid %d >= best ask %d", symbol, bid.Price, ask.Price)
		}
	}
}
