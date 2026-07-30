package api

import (
	"context"
	"io"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/engine"
)

const bufSize = 1024 * 1024

func newTestClient(t *testing.T) pb.ExchangeClient {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	exchange := engine.NewExchange()
	pb.RegisterExchangeServer(grpcServer, NewServer(exchange))

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

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return pb.NewExchangeClient(conn)
}

func TestServer_SubmitOrderMatches(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	sellResp, err := client.SubmitOrder(ctx, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_SELL, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 50,
	})
	if err != nil {
		t.Fatalf("SubmitOrder (sell) returned error: %v", err)
	}
	if len(sellResp.Trades) != 0 {
		t.Fatalf("resting sell produced trades: %+v", sellResp.Trades)
	}

	buyResp, err := client.SubmitOrder(ctx, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 20,
	})
	if err != nil {
		t.Fatalf("SubmitOrder (buy) returned error: %v", err)
	}
	if len(buyResp.Trades) != 1 || buyResp.Trades[0].Quantity != 20 {
		t.Fatalf("trades = %+v, want one trade of qty 20", buyResp.Trades)
	}
	if buyResp.Trades[0].MakerOrderId != sellResp.Order.Id {
		t.Fatalf("MakerOrderId = %d, want %d", buyResp.Trades[0].MakerOrderId, sellResp.Order.Id)
	}
}

func TestServer_SubmitOrderRejectsCancelledContext(t *testing.T) {
	exchange := engine.NewExchange()
	defer exchange.Close()
	srv := NewServer(exchange)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := srv.SubmitOrder(ctx, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 10,
	})

	if status.Code(err) != codes.Canceled {
		t.Fatalf("status code = %v, want Canceled (err: %v)", status.Code(err), err)
	}
}

func TestServer_SubmitOrderRejectsInvalidQuantity(t *testing.T) {
	client := newTestClient(t)

	_, err := client.SubmitOrder(context.Background(), &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 0,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("status code = %v, want InvalidArgument (err: %v)", status.Code(err), err)
	}
}

func TestServer_CancelOrder(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	submitResp, err := client.SubmitOrder(ctx, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_SELL, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 50,
	})
	if err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	cancelResp, err := client.CancelOrder(ctx, &pb.CancelOrderRequest{
		Symbol: "AAPL", OrderId: submitResp.Order.Id,
	})
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}
	if cancelResp.Order.Remaining != 50 {
		t.Fatalf("cancelled order Remaining = %d, want 50", cancelResp.Order.Remaining)
	}
}

func TestServer_CancelOrderNotFound(t *testing.T) {
	client := newTestClient(t)

	_, err := client.CancelOrder(context.Background(), &pb.CancelOrderRequest{
		Symbol: "AAPL", OrderId: 999,
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("status code = %v, want NotFound (err: %v)", status.Code(err), err)
	}
}

func TestServer_GetOrderBook(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	if _, err := client.SubmitOrder(ctx, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 10,
	}); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}
	if _, err := client.SubmitOrder(ctx, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_SELL, Type: pb.OrderType_LIMIT, Price: 105, Quantity: 5,
	}); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	book, err := client.GetOrderBook(ctx, &pb.GetOrderBookRequest{Symbol: "AAPL", Depth: 10})
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

func TestServer_GetOrderBookRejectsNegativeDepth(t *testing.T) {
	client := newTestClient(t)

	_, err := client.GetOrderBook(context.Background(), &pb.GetOrderBookRequest{Symbol: "AAPL", Depth: -1})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("status code = %v, want InvalidArgument (err: %v)", status.Code(err), err)
	}
}

func TestServer_SubscribeTradesReceivesLiveTrades(t *testing.T) {
	client := newTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := client.SubscribeTrades(ctx, &pb.SubscribeTradesRequest{Symbol: "AAPL"})
	if err != nil {
		t.Fatalf("SubscribeTrades returned error: %v", err)
	}

	if _, err := client.SubmitOrder(context.Background(), &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_SELL, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 10,
	}); err != nil {
		t.Fatalf("SubmitOrder (sell) returned error: %v", err)
	}
	if _, err := client.SubmitOrder(context.Background(), &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 10,
	}); err != nil {
		t.Fatalf("SubmitOrder (buy) returned error: %v", err)
	}

	tr, err := stream.Recv()
	if err == io.EOF {
		t.Fatalf("stream closed before receiving a trade")
	}
	if err != nil {
		t.Fatalf("stream.Recv() returned error: %v", err)
	}
	if tr.Quantity != 10 || tr.Symbol != "AAPL" {
		t.Fatalf("trade = %+v, want qty 10 on AAPL", tr)
	}
}
