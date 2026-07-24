package client

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/guilhermemcandido/janus/internal/api"
	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/engine"
)

const bufSize = 1024 * 1024

func newTestClient(t *testing.T) *Client {
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

	c, err := Dial("passthrough:///bufnet", grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	return c
}

func TestClient_SubmitOrderMatches(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	sellOrder, sellTrades, err := c.SubmitOrder(ctx, "AAPL", Sell, Limit, 100, 50)
	if err != nil {
		t.Fatalf("SubmitOrder (sell) returned error: %v", err)
	}
	if len(sellTrades) != 0 {
		t.Fatalf("resting sell produced trades: %+v", sellTrades)
	}

	_, buyTrades, err := c.SubmitOrder(ctx, "AAPL", Buy, Limit, 100, 20)
	if err != nil {
		t.Fatalf("SubmitOrder (buy) returned error: %v", err)
	}
	if len(buyTrades) != 1 || buyTrades[0].Quantity != 20 {
		t.Fatalf("trades = %+v, want one trade of qty 20", buyTrades)
	}
	if buyTrades[0].MakerOrderID != sellOrder.ID {
		t.Fatalf("MakerOrderID = %d, want %d", buyTrades[0].MakerOrderID, sellOrder.ID)
	}
}

func TestClient_CancelOrder(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	order, _, err := c.SubmitOrder(ctx, "AAPL", Sell, Limit, 100, 50)
	if err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	cancelled, err := c.CancelOrder(ctx, "AAPL", order.ID)
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}
	if cancelled.Remaining != 50 {
		t.Fatalf("cancelled Remaining = %d, want 50", cancelled.Remaining)
	}
}

func TestClient_CancelOrderNotFoundPropagatesGRPCStatus(t *testing.T) {
	c := newTestClient(t)

	_, err := c.CancelOrder(context.Background(), "AAPL", 999)
	if status.Code(err) != codes.NotFound {
		t.Fatalf("status code = %v, want NotFound (err: %v)", status.Code(err), err)
	}
}

func TestClient_GetOrderBook(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	if _, _, err := c.SubmitOrder(ctx, "AAPL", Buy, Limit, 100, 10); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}
	if _, _, err := c.SubmitOrder(ctx, "AAPL", Sell, Limit, 105, 5); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	book, err := c.GetOrderBook(ctx, "AAPL", 10)
	if err != nil {
		t.Fatalf("GetOrderBook returned error: %v", err)
	}
	if len(book.Bids) != 1 || book.Bids[0].Price != 100 || book.Bids[0].Quantity != 10 {
		t.Fatalf("Bids = %+v, want [{100 10}]", book.Bids)
	}
	if len(book.Asks) != 1 || book.Asks[0].Price != 105 || book.Asks[0].Quantity != 5 {
		t.Fatalf("Asks = %+v, want [{105 5}]", book.Asks)
	}
}

func TestClient_SubscribeTradesReceivesLiveTrades(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trades, err := c.SubscribeTrades(ctx, "AAPL")
	if err != nil {
		t.Fatalf("SubscribeTrades returned error: %v", err)
	}

	if _, _, err := c.SubmitOrder(context.Background(), "AAPL", Sell, Limit, 100, 10); err != nil {
		t.Fatalf("SubmitOrder (sell) returned error: %v", err)
	}
	if _, _, err := c.SubmitOrder(context.Background(), "AAPL", Buy, Limit, 100, 10); err != nil {
		t.Fatalf("SubmitOrder (buy) returned error: %v", err)
	}

	select {
	case tr := <-trades:
		if tr.Quantity != 10 || tr.Symbol != "AAPL" {
			t.Fatalf("trade = %+v, want qty 10 on AAPL", tr)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected to receive a trade on the subscription channel")
	}
}
