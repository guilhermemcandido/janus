package api

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
)

const benchNumSymbols = 8

func registerBenchSymbols(b *testing.B, client pb.ExchangeClient) []string {
	b.Helper()
	symbols := make([]string, benchNumSymbols)
	for i := range symbols {
		symbols[i] = fmt.Sprintf("BENCH%d", i)
		registerMarket(b, client, symbols[i], "")
	}
	return symbols
}

// BenchmarkSubmitOrderRoundTrip measures SubmitOrder over real gRPC (bufconn), concurrently across
// several markets - a sequential loop would measure one client's latency, not real server throughput.
func BenchmarkSubmitOrderRoundTrip(b *testing.B) {
	client := newTestClient(b)
	ctx := context.Background()
	symbols := registerBenchSymbols(b, client)

	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		i := 0
		for p.Next() {
			symbol := symbols[i%benchNumSymbols]
			i++
			if _, err := client.SubmitOrder(ctx, &pb.SubmitOrderRequest{
				Symbol: symbol, Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 1,
			}); err != nil {
				b.Fatalf("SubmitOrder: %v", err)
			}
		}
	})
}

// BenchmarkCancelOrderRoundTrip measures CancelOrder the same way, concurrently over real gRPC,
// each goroutine cancelling its own distinct pre-submitted order claimed via an atomic counter.
func BenchmarkCancelOrderRoundTrip(b *testing.B) {
	client := newTestClient(b)
	ctx := context.Background()
	symbols := registerBenchSymbols(b, client)

	type resting struct {
		symbol string
		id     uint64
	}
	orders := make([]resting, b.N)
	for i := range orders {
		symbol := symbols[i%benchNumSymbols]
		resp, err := client.SubmitOrder(ctx, &pb.SubmitOrderRequest{
			Symbol: symbol, Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: int64(50 + i%200), Quantity: 1,
		})
		if err != nil {
			b.Fatalf("SubmitOrder: %v", err)
		}
		orders[i] = resting{symbol: symbol, id: resp.Order.Id}
	}

	var next atomic.Int64
	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			o := orders[next.Add(1)-1]
			if _, err := client.CancelOrder(ctx, &pb.CancelOrderRequest{Symbol: o.symbol, OrderId: o.id}); err != nil {
				b.Fatalf("CancelOrder: %v", err)
			}
		}
	})
}
