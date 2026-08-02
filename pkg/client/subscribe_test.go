package client

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/guilhermemcandido/janus/internal/api"
	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/engine"
)

// submitWithRetry retries SubmitOrder until it succeeds or deadline passes, tolerating the
// brief window where a client's channel hasn't yet reconnected after a server restart.
func submitWithRetry(t *testing.T, c *Client, symbol string, side Side, price int64, qty uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, _, err := c.SubmitOrder(context.Background(), symbol, side, Limit, price, qty); err == nil {
			return
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("SubmitOrder never succeeded: %v", lastErr)
}

func TestClient_SubscribeTradesReconnectsAfterServerRestart(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()

	exchange1 := engine.NewExchange()
	exchange1.Register("AAPL", "Apple Inc.")
	srv1 := grpc.NewServer()
	pb.RegisterExchangeServer(srv1, api.NewServer(exchange1))
	go func() { _ = srv1.Serve(lis) }()

	c, err := Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trades, err := c.SubscribeTrades(ctx, "AAPL")
	if err != nil {
		t.Fatalf("SubscribeTrades returned error: %v", err)
	}

	submitWithRetry(t, c, "AAPL", Sell, 100, 10)
	submitWithRetry(t, c, "AAPL", Buy, 100, 10)

	select {
	case <-trades:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected an initial trade before restarting the server")
	}

	// Kill the server (simulating a crash) and start a fresh one on the same address.
	srv1.Stop()
	exchange1.Close()

	exchange2 := engine.NewExchange()
	exchange2.Register("AAPL", "Apple Inc.")
	lis2, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("relisten on %s: %v", addr, err)
	}
	srv2 := grpc.NewServer()
	pb.RegisterExchangeServer(srv2, api.NewServer(exchange2))
	go func() { _ = srv2.Serve(lis2) }()
	defer srv2.Stop()
	defer exchange2.Close()

	// A trade fired right as the server comes back can still race the resubscribe and get missed
	// (acceptable, same as any pub/sub without replay) - retry until one lands post-reconnect.
	var trade Trade
	received := false
	for attempt := int64(0); attempt < 10 && !received; attempt++ {
		price := 200 + attempt
		submitWithRetry(t, c, "AAPL", Sell, price, 5)
		submitWithRetry(t, c, "AAPL", Buy, price, 5)

		select {
		case trade = <-trades:
			received = true
		case <-time.After(300 * time.Millisecond):
		}
	}
	if !received {
		t.Fatalf("expected the subscription to recover and deliver a trade after the server restarted")
	}
	if trade.Price < 200 {
		t.Fatalf("trade = %+v, want price >= 200 (from the restarted exchange)", trade)
	}
}
