package api

import (
	"context"
	"testing"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
)

// BenchmarkSubmitOrderRoundTrip measures a full SubmitOrder call over an in-process gRPC connection
// (bufconn), the same client/server stack real clients use - not just the engine underneath it.
func BenchmarkSubmitOrderRoundTrip(b *testing.B) {
	client := newTestClient(b)
	ctx := context.Background()
	registerMarket(b, client, "BENCH", "")

	req := &pb.SubmitOrderRequest{
		Symbol: "BENCH", Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := client.SubmitOrder(ctx, req); err != nil {
			b.Fatalf("SubmitOrder: %v", err)
		}
	}
}
