package hedger

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

// seedLiquidity rests deep, wide-spread bid/ask on symbol so small market orders always fill.
func seedLiquidity(t *testing.T, c *client.Client, symbol string) {
	t.Helper()
	if _, _, err := c.SubmitOrder(context.Background(), symbol, client.Buy, client.Limit, 90, 100000); err != nil {
		t.Fatalf("seed bid: %v", err)
	}
	if _, _, err := c.SubmitOrder(context.Background(), symbol, client.Sell, client.Limit, 110, 100000); err != nil {
		t.Fatalf("seed ask: %v", err)
	}
}

func testConfig() Config {
	return Config{
		FuturesSymbol:  "AAPLF",
		SpotSymbol:     "AAPL",
		FlowQuantity:   5,
		Interval:       time.Second,
		HedgeThreshold: 10,
	}
}

func TestHedger_NeverExceedsThresholdAfterAct(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	cfg := testConfig()
	seedLiquidity(t, c, cfg.FuturesSymbol)
	seedLiquidity(t, c, cfg.SpotSymbol)

	h := New(c, cfg)
	for i := 0; i < 50; i++ {
		if err := h.Act(ctx, &bytes.Buffer{}); err != nil {
			t.Fatalf("Act returned error: %v", err)
		}
		if h.position > cfg.HedgeThreshold || h.position < -cfg.HedgeThreshold {
			t.Fatalf("position = %d after Act, want within +/-%d", h.position, cfg.HedgeThreshold)
		}
	}
}

func TestHedger_HedgeIfNeededFlattensPosition(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	cfg := testConfig()
	seedLiquidity(t, c, cfg.SpotSymbol)

	h := New(c, cfg)
	h.position = 15

	h.hedgeIfNeeded(ctx, &bytes.Buffer{})

	if h.position != 0 {
		t.Fatalf("position = %d after hedge, want 0", h.position)
	}
}

func TestHedger_ActFailedTracksSubmitError(t *testing.T) {
	c := newTestClient(t)
	h := New(c, testConfig())

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.Act(cancelledCtx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

	if !h.ActFailed() {
		t.Fatalf("ActFailed() = false after a cancelled-context submit, want true")
	}
}

func TestHedger_ResetClearsPosition(t *testing.T) {
	c := newTestClient(t)
	h := New(c, testConfig())
	h.position = 15

	h.Reset(context.Background(), &bytes.Buffer{})

	if h.position != 0 {
		t.Fatalf("position = %d after Reset, want 0", h.position)
	}
}

func TestHedger_HedgeIfNeededFlattensShortPosition(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	cfg := testConfig()
	seedLiquidity(t, c, cfg.SpotSymbol)

	h := New(c, cfg)
	h.position = -15

	h.hedgeIfNeeded(ctx, &bytes.Buffer{})

	if h.position != 0 {
		t.Fatalf("position = %d after hedge, want 0", h.position)
	}
}
