package client

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/guilhermemcandido/janus/internal/api"
	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/engine"
)

// TestClient_ConcurrentClientsRecoverFromServerRestart proves the reconnection story holds under
// real concurrent load: several independent clients hammering the same server, not just one.
func TestClient_ConcurrentClientsRecoverFromServerRestart(t *testing.T) {
	const numClients = 5

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()

	exchange1 := engine.NewExchange()
	srv1 := grpc.NewServer()
	pb.RegisterExchangeServer(srv1, api.NewServer(exchange1))
	go func() { _ = srv1.Serve(lis) }()

	clients := make([]*Client, numClients)
	for i := range clients {
		c, err := Dial(addr)
		if err != nil {
			t.Fatalf("Dial client %d: %v", i, err)
		}
		defer c.Close()
		clients[i] = c
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stop := make(chan struct{})

	var restarted atomic.Bool
	before := make([]int64, numClients)
	after := make([]int64, numClients)

	// Each client trades its own exclusive symbol: a shared symbol wouldn't isolate clients, since
	// a crossing limit order can reach any resting order at or better than its price, not just its own.
	var wg sync.WaitGroup
	for i, c := range clients {
		wg.Add(1)
		go func(i int, c *Client) {
			defer wg.Done()
			symbol := fmt.Sprintf("SYM%d", i)
			for {
				select {
				case <-stop:
					return
				default:
				}
				// Checked only between complete round trips: cancelling an in-flight call could
				// still land server-side, stranding an order with no partner to cross it.
				if _, _, err := c.SubmitOrder(ctx, symbol, Buy, Limit, 100, 5); err != nil {
					time.Sleep(20 * time.Millisecond)
					continue
				}
				sellDeadline := time.Now().Add(5 * time.Second)
				for {
					if _, _, err := c.SubmitOrder(ctx, symbol, Sell, Limit, 100, 5); err == nil {
						break
					}
					if time.Now().After(sellDeadline) {
						t.Errorf("client %d: sell leg never completed, resting buy may be stranded", i)
						return
					}
					time.Sleep(20 * time.Millisecond)
				}
				if restarted.Load() {
					after[i]++
				} else {
					before[i]++
				}
				time.Sleep(10 * time.Millisecond)
			}
		}(i, c)
	}

	time.Sleep(500 * time.Millisecond) // let every client rack up some pre-restart round trips

	srv1.Stop()
	exchange1.Close()

	exchange2 := engine.NewExchange()
	lis2, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("relisten on %s: %v", addr, err)
	}
	srv2 := grpc.NewServer()
	pb.RegisterExchangeServer(srv2, api.NewServer(exchange2))
	go func() { _ = srv2.Serve(lis2) }()
	defer srv2.Stop()
	defer exchange2.Close()
	restarted.Store(true)

	time.Sleep(2 * time.Second) // give every client time to hit Unavailable and reconnect on its own
	close(stop)
	wg.Wait()

	for i := range clients {
		if before[i] == 0 {
			t.Errorf("client %d: no successful round trips before the restart", i)
		}
		if after[i] == 0 {
			t.Errorf("client %d: no successful round trips after the restart - did it fail to reconnect?", i)
		}
	}

	// A retried leg whose earlier attempt actually succeeded on the discarded old exchange finds
	// nothing to cross on the fresh one - an inherent ambiguous-outcome race, not a bug, capped at one stray order.
	for i, c := range clients {
		symbol := fmt.Sprintf("SYM%d", i)
		book, err := c.GetOrderBook(context.Background(), symbol, 100)
		if err != nil {
			t.Fatalf("GetOrderBook returned error: %v", err)
		}
		var resting uint64
		for _, lvl := range book.Bids {
			resting += lvl.Quantity
		}
		for _, lvl := range book.Asks {
			resting += lvl.Quantity
		}
		if resting > 5 {
			t.Fatalf("client %d book = %+v, want at most one stray order (qty <= 5)", i, book)
		}
	}
}
