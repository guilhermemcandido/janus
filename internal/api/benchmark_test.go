package api

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/engine"
	"github.com/guilhermemcandido/janus/pkg/client"
)

const benchNumSymbols = 8

// newBenchClient uses pkg/client, not the raw stub - benchmarking the real path bots/CLI/web use,
// including the shared OrderStream, not a hand-rolled shortcut through it.
func newBenchClient(b *testing.B) *client.Client {
	b.Helper()
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	exchange := engine.NewExchange()
	pb.RegisterExchangeServer(grpcServer, NewServer(exchange))
	go func() { _ = grpcServer.Serve(lis) }()
	b.Cleanup(func() {
		grpcServer.Stop()
		exchange.Close()
	})

	dialer := func(ctx context.Context, _ string) (net.Conn, error) { return lis.Dial() }
	c, err := client.Dial("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		b.Fatalf("client.Dial: %v", err)
	}
	b.Cleanup(func() { c.Close() })
	return c
}

func registerBenchSymbols(b *testing.B, c *client.Client) []string {
	b.Helper()
	symbols := make([]string, benchNumSymbols)
	for i := range symbols {
		symbols[i] = fmt.Sprintf("BENCH%d", i)
		if _, err := c.RegisterMarket(context.Background(), symbols[i], ""); err != nil {
			b.Fatalf("RegisterMarket: %v", err)
		}
	}
	return symbols
}

// BenchmarkSubmitOrderRoundTrip measures SubmitOrder over real gRPC (bufconn), concurrently across
// several markets, all sharing one Client's OrderStream - the shape real usage actually takes.
func BenchmarkSubmitOrderRoundTrip(b *testing.B) {
	c := newBenchClient(b)
	ctx := context.Background()
	symbols := registerBenchSymbols(b, c)

	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		i := 0
		for p.Next() {
			symbol := symbols[i%benchNumSymbols]
			i++
			if _, _, err := c.SubmitOrder(ctx, symbol, client.Buy, client.Limit, 100, 1); err != nil {
				b.Fatalf("SubmitOrder: %v", err)
			}
		}
	})
}

// BenchmarkCancelOrderRoundTrip measures CancelOrder the same way, each goroutine cancelling its own
// distinct pre-submitted order claimed via an atomic counter.
func BenchmarkCancelOrderRoundTrip(b *testing.B) {
	c := newBenchClient(b)
	ctx := context.Background()
	symbols := registerBenchSymbols(b, c)

	type resting struct {
		symbol string
		id     uint64
	}
	orders := make([]resting, b.N)
	for i := range orders {
		symbol := symbols[i%benchNumSymbols]
		order, _, err := c.SubmitOrder(ctx, symbol, client.Buy, client.Limit, int64(50+i%200), 1)
		if err != nil {
			b.Fatalf("SubmitOrder: %v", err)
		}
		orders[i] = resting{symbol: symbol, id: order.ID}
	}

	var next atomic.Int64
	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			o := orders[next.Add(1)-1]
			if _, err := c.CancelOrder(ctx, o.symbol, o.id); err != nil {
				b.Fatalf("CancelOrder: %v", err)
			}
		}
	})
}
