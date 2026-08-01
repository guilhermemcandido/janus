package arbitrage

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

	return c
}

func seedBook(t *testing.T, c *client.Client, symbol string, bid, ask int64) {
	t.Helper()
	if _, _, err := c.SubmitOrder(context.Background(), symbol, client.Buy, client.Limit, bid, 100000); err != nil {
		t.Fatalf("seed bid: %v", err)
	}
	if _, _, err := c.SubmitOrder(context.Background(), symbol, client.Sell, client.Limit, ask, 100000); err != nil {
		t.Fatalf("seed ask: %v", err)
	}
}

func testConfig() Config {
	return Config{
		SpotSymbol:     "AAPL",
		FuturesSymbol:  "AAPLF",
		FairBasis:      5,
		EntryThreshold: 4,
		Quantity:       10,
		Interval:       time.Second,
	}
}

func TestArbitrage_EntersShortBasisWhenFuturesRich(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	// spot mid = 100, futures mid = 112 -> spread 12, fair basis 5, deviation 7 > threshold 4.
	seedBook(t, c, "AAPL", 99, 101)
	seedBook(t, c, "AAPLF", 111, 113)

	a := New(c, testConfig())
	if err := a.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

	if !a.inPosition || a.long {
		t.Fatalf("inPosition=%v long=%v, want inPosition=true long=false (short-basis)", a.inPosition, a.long)
	}
}

func TestArbitrage_EntersLongBasisWhenFuturesCheap(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	// spot mid = 100, futures mid = 98 -> spread -2, fair basis 5, deviation -7 < -threshold.
	seedBook(t, c, "AAPL", 99, 101)
	seedBook(t, c, "AAPLF", 97, 99)

	a := New(c, testConfig())
	if err := a.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

	if !a.inPosition || !a.long {
		t.Fatalf("inPosition=%v long=%v, want inPosition=true long=true (long-basis)", a.inPosition, a.long)
	}
}

func TestArbitrage_StaysFlatWithinThreshold(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	// spot mid = 100, futures mid = 104 -> spread 4, fair basis 5, deviation -1, within +/-4.
	seedBook(t, c, "AAPL", 99, 101)
	seedBook(t, c, "AAPLF", 103, 105)

	a := New(c, testConfig())
	if err := a.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

	if a.inPosition {
		t.Fatalf("inPosition = true, want false (deviation within threshold)")
	}
}

func TestArbitrage_ExitsOncePositionRevertsHalfway(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	cfg := testConfig()
	a := New(c, cfg)
	a.inPosition = true
	a.long = true

	// exit band is +/- EntryThreshold/2 = +/-2; deviation of 0 is within it.
	seedBook(t, c, "AAPL", 99, 101)
	seedBook(t, c, "AAPLF", 104, 106) // mid 105, fair basis 5 -> deviation 0

	if err := a.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}
	if a.inPosition {
		t.Fatalf("inPosition = true, want false (deviation reverted within exit band)")
	}
}

func TestArbitrage_ActFailedTracksBookRPCError(t *testing.T) {
	c := newTestClient(t)
	a := New(c, testConfig())

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Act(cancelledCtx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

	if !a.ActFailed() {
		t.Fatalf("ActFailed() = false after a cancelled-context book fetch, want true")
	}
}

func TestArbitrage_ActNotFailedWhenBookMerelyIncomplete(t *testing.T) {
	c := newTestClient(t)
	seedBook(t, c, "AAPL", 99, 101)
	// AAPLF has no resting orders, so mid() returns ok=false with no RPC error.

	a := New(c, testConfig())
	if err := a.Act(context.Background(), &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

	if a.ActFailed() {
		t.Fatalf("ActFailed() = true for a merely-incomplete book, want false (no RPC error occurred)")
	}
}

func TestArbitrage_ResetClearsPosition(t *testing.T) {
	c := newTestClient(t)
	a := New(c, testConfig())
	a.inPosition, a.long = true, true

	a.Reset(context.Background(), &bytes.Buffer{})

	if a.inPosition {
		t.Fatalf("inPosition = true after Reset, want false")
	}
}

func TestArbitrage_SkipsWhenBookIncomplete(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	seedBook(t, c, "AAPL", 99, 101)
	// AAPLF has no resting orders at all.

	a := New(c, testConfig())
	if err := a.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}
	if a.inPosition {
		t.Fatalf("inPosition = true, want false (should skip when a book side is missing)")
	}
}
