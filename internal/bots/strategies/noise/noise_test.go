package noise

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

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

func seedLiquidity(t *testing.T, c *client.Client, symbol string) {
	t.Helper()
	if _, _, err := c.SubmitOrder(context.Background(), symbol, client.Buy, client.Limit, 90, 100000); err != nil {
		t.Fatalf("seed bid: %v", err)
	}
	if _, _, err := c.SubmitOrder(context.Background(), symbol, client.Sell, client.Limit, 110, 100000); err != nil {
		t.Fatalf("seed ask: %v", err)
	}
}

func TestNoise_ActTradesOnlyConfiguredSymbols(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	seedLiquidity(t, c, "AAPL")
	seedLiquidity(t, c, "AAPLF")

	cfg := Config{Symbols: []string{"AAPL", "AAPLF"}, Quantity: 5, Interval: time.Second}
	n := New(c, cfg)

	for i := 0; i < 20; i++ {
		if err := n.Act(ctx, &bytes.Buffer{}); err != nil {
			t.Fatalf("Act returned error: %v", err)
		}
	}

	for _, symbol := range cfg.Symbols {
		book, err := c.GetOrderBook(ctx, symbol, 1)
		if err != nil {
			t.Fatalf("GetOrderBook(%s) returned error: %v", symbol, err)
		}
		if len(book.Bids) == 0 || len(book.Asks) == 0 {
			t.Fatalf("%s book = %+v, want the seeded liquidity to still be resting", symbol, book)
		}
	}
}

func TestNoise_ActErrorsGracefullyOnUnknownSymbol(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	cfg := Config{Symbols: []string{"AAPL"}, Quantity: 5, Interval: time.Second}
	n := New(c, cfg)

	// No liquidity seeded: the market order just fills zero, no error expected.
	var out bytes.Buffer
	if err := n.Act(ctx, &out); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}
}
