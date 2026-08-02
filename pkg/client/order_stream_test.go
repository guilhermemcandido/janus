package client

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestClient_ConcurrentOrdersOnOneClientDontCrossWires proves correlation IDs, not call order, match
// replies to callers - every goroutine here shares one Client, like internal/web shares one.
func TestClient_ConcurrentOrdersOnOneClientDontCrossWires(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	const numSymbols = 20
	symbols := make([]string, numSymbols)
	for i := range symbols {
		symbols[i] = fmt.Sprintf("SYM%d", i)
		if _, err := c.RegisterMarket(ctx, symbols[i], symbols[i]); err != nil {
			t.Fatalf("RegisterMarket(%s): %v", symbols[i], err)
		}
	}

	var wg sync.WaitGroup
	for i, symbol := range symbols {
		wg.Add(1)
		go func(i int, symbol string) {
			defer wg.Done()
			price := int64(100 + i)

			order, _, err := c.SubmitOrder(ctx, symbol, Buy, Limit, price, 5)
			if err != nil {
				t.Errorf("SubmitOrder(%s): %v", symbol, err)
				return
			}
			if order.Symbol != symbol || order.Price != price {
				t.Errorf("SubmitOrder(%s) returned order for %s @ %d, want %s @ %d", symbol, order.Symbol, order.Price, symbol, price)
				return
			}

			cancelled, err := c.CancelOrder(ctx, symbol, order.ID)
			if err != nil {
				t.Errorf("CancelOrder(%s): %v", symbol, err)
				return
			}
			if cancelled.ID != order.ID || cancelled.Symbol != symbol {
				t.Errorf("CancelOrder(%s) returned order %+v, want ID=%d Symbol=%s", symbol, cancelled, order.ID, symbol)
			}
		}(i, symbol)
	}
	wg.Wait()
}

// TestClient_PendingOrderFailsPromptlyWhenServerStops proves a Submit blocked waiting for its reply
// returns quickly with an error when the stream breaks, rather than hanging until ctx's own deadline.
func TestClient_PendingOrderFailsPromptlyWhenServerStops(t *testing.T) {
	c, stop := newStoppableTestClient(t)
	ctx := context.Background()
	if _, err := c.RegisterMarket(ctx, "AAPL", "Apple Inc."); err != nil {
		t.Fatalf("RegisterMarket: %v", err)
	}

	stop()

	done := make(chan error, 1)
	go func() {
		_, _, err := c.SubmitOrder(ctx, "AAPL", Buy, Limit, 100, 1)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("SubmitOrder after server stopped = nil error, want a failure")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("SubmitOrder never returned after the server stopped")
	}
}
