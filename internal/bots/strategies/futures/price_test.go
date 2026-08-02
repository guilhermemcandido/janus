package futures

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/guilhermemcandido/janus/internal/api"
	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/engine"
	"github.com/guilhermemcandido/janus/pkg/client"
)

const bufSize = 1024 * 1024

func newTestClient(t *testing.T) *client.Client {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	exchange := engine.NewExchange()
	pb.RegisterExchangeServer(grpcServer, api.NewServer(exchange))

	go func() {
		_ = grpcServer.Serve(lis)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		exchange.Close()
	})

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.Dial()
	}

	c, err := client.Dial("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	for _, symbol := range []string{"AAPL", "AAPLF"} {
		if _, err := c.RegisterMarket(context.Background(), symbol, symbol); err != nil {
			t.Fatalf("RegisterMarket(%q): %v", symbol, err)
		}
	}

	return c
}

func TestSpotMidPriceSource_UsesFallbackWhenBookIsEmpty(t *testing.T) {
	c := newTestClient(t)
	src := NewSpotMidPriceSource(c, "AAPL", 5, 100)

	price, err := src.Price(context.Background())
	if err != nil {
		t.Fatalf("Price returned error: %v", err)
	}
	if price != 100 {
		t.Fatalf("Price = %d, want fallback 100", price)
	}
}

func TestSpotMidPriceSource_UsesFallbackWhenOnlyOneSideResting(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	if _, _, err := c.SubmitOrder(ctx, "AAPL", client.Buy, client.Limit, 98, 10); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	src := NewSpotMidPriceSource(c, "AAPL", 5, 100)
	price, err := src.Price(ctx)
	if err != nil {
		t.Fatalf("Price returned error: %v", err)
	}
	if price != 100 {
		t.Fatalf("Price = %d, want fallback 100 (only one side resting)", price)
	}
}

func TestSpotMidPriceSource_TracksSpotMidPlusBasis(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	if _, _, err := c.SubmitOrder(ctx, "AAPL", client.Buy, client.Limit, 98, 10); err != nil {
		t.Fatalf("SubmitOrder (bid) returned error: %v", err)
	}
	if _, _, err := c.SubmitOrder(ctx, "AAPL", client.Sell, client.Limit, 102, 10); err != nil {
		t.Fatalf("SubmitOrder (ask) returned error: %v", err)
	}

	src := NewSpotMidPriceSource(c, "AAPL", 5, 999)
	price, err := src.Price(ctx)
	if err != nil {
		t.Fatalf("Price returned error: %v", err)
	}
	// mid = (98+102)/2 = 100, + basis 5 = 105
	if price != 105 {
		t.Fatalf("Price = %d, want 105 (mid 100 + basis 5)", price)
	}
}

func TestSpotMidPriceSource_StaysAtLastGoodPriceOnGap(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	if _, _, err := c.SubmitOrder(ctx, "AAPL", client.Buy, client.Limit, 98, 10); err != nil {
		t.Fatalf("SubmitOrder (bid) returned error: %v", err)
	}
	askResp, _, err := c.SubmitOrder(ctx, "AAPL", client.Sell, client.Limit, 102, 10)
	if err != nil {
		t.Fatalf("SubmitOrder (ask) returned error: %v", err)
	}

	src := NewSpotMidPriceSource(c, "AAPL", 5, 999)
	price, err := src.Price(ctx)
	if err != nil {
		t.Fatalf("Price returned error: %v", err)
	}
	if price != 105 {
		t.Fatalf("Price = %d, want 105 (mid 100 + basis 5)", price)
	}

	// Simulate a momentary gap in the spot book, like the spot bot mid-requote.
	if _, err := c.CancelOrder(ctx, "AAPL", askResp.ID); err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}

	price, err = src.Price(ctx)
	if err != nil {
		t.Fatalf("Price returned error: %v", err)
	}
	if price != 105 {
		t.Fatalf("Price = %d, want 105 (sticky last known-good value, not the fixed initial 999)", price)
	}
}
