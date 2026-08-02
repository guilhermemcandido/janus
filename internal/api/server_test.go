package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
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

func newTestClient(t testing.TB) pb.ExchangeClient {
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

func registerMarket(t testing.TB, client pb.ExchangeClient, symbol, description string) {
	t.Helper()
	if _, err := client.RegisterMarket(context.Background(), &pb.RegisterMarketRequest{
		Symbol: symbol, Description: description,
	}); err != nil {
		t.Fatalf("RegisterMarket(%q) returned error: %v", symbol, err)
	}
}

// submitOrder/cancelOrder speak the OrderStream RPC for tests that used the old unary calls. No
// t.Fatalf here - some callers use these from a goroutine other than the test's own.
func submitOrder(t testing.TB, client pb.ExchangeClient, req *pb.SubmitOrderRequest) (*pb.SubmitOrderResponse, error) {
	t.Helper()
	evt, err := sendOrderCommand(client, &pb.OrderCommand{Command: &pb.OrderCommand_Submit{Submit: req}})
	if err != nil {
		return nil, err
	}
	switch e := evt.Event.(type) {
	case *pb.OrderEvent_SubmitResult:
		return e.SubmitResult, nil
	case *pb.OrderEvent_Error:
		return nil, status.Error(codes.Code(e.Error.Code), e.Error.Message)
	default:
		return nil, fmt.Errorf("unexpected OrderEvent type %T", evt.Event)
	}
}

func cancelOrder(t testing.TB, client pb.ExchangeClient, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
	t.Helper()
	evt, err := sendOrderCommand(client, &pb.OrderCommand{Command: &pb.OrderCommand_Cancel{Cancel: req}})
	if err != nil {
		return nil, err
	}
	switch e := evt.Event.(type) {
	case *pb.OrderEvent_CancelResult:
		return e.CancelResult, nil
	case *pb.OrderEvent_Error:
		return nil, status.Error(codes.Code(e.Error.Code), e.Error.Message)
	default:
		return nil, fmt.Errorf("unexpected OrderEvent type %T", evt.Event)
	}
}

// sendOrderCommand opens a fresh OrderStream, sends one command, and returns its matching reply.
func sendOrderCommand(client pb.ExchangeClient, cmd *pb.OrderCommand) (*pb.OrderEvent, error) {
	stream, err := client.OrderStream(context.Background())
	if err != nil {
		return nil, err
	}
	cmd.CorrelationId = 1
	if err := stream.Send(cmd); err != nil {
		return nil, err
	}
	return stream.Recv()
}

func TestServer_RegisterMarketIsIdempotent(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	registerMarket(t, client, "AAPL", "Apple Inc.")
	resp, err := client.RegisterMarket(ctx, &pb.RegisterMarketRequest{Symbol: "AAPL", Description: "something else"})
	if err != nil {
		t.Fatalf("second RegisterMarket returned error: %v", err)
	}
	if resp.Market.Description != "Apple Inc." {
		t.Fatalf("Description = %q, want the first registration's description to win", resp.Market.Description)
	}
}

func TestServer_RegisterMarketRejectsEmptySymbol(t *testing.T) {
	client := newTestClient(t)

	_, err := client.RegisterMarket(context.Background(), &pb.RegisterMarketRequest{Symbol: "", Description: "x"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("status code = %v, want InvalidArgument (err: %v)", status.Code(err), err)
	}
}

func TestServer_RegisterMarketRejectsWhitespaceOnlySymbol(t *testing.T) {
	client := newTestClient(t)

	_, err := client.RegisterMarket(context.Background(), &pb.RegisterMarketRequest{Symbol: "   ", Description: "x"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("status code = %v, want InvalidArgument (err: %v)", status.Code(err), err)
	}
}

func TestServer_RegisterMarketNormalizesSymbolCase(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	resp, err := client.RegisterMarket(ctx, &pb.RegisterMarketRequest{Symbol: "aapl", Description: "Apple Inc."})
	if err != nil {
		t.Fatalf("RegisterMarket returned error: %v", err)
	}
	if resp.Market.Symbol != "AAPL" {
		t.Fatalf("Symbol = %q, want upper-cased %q", resp.Market.Symbol, "AAPL")
	}

	if _, err := submitOrder(t, client, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 10,
	}); err != nil {
		t.Fatalf("SubmitOrder(\"AAPL\") returned error: %v, want the lower-cased registration to be reachable in upper case", err)
	}
}

func TestServer_SubmitOrderMatches(t *testing.T) {
	client := newTestClient(t)
	registerMarket(t, client, "AAPL", "Apple Inc.")

	sellResp, err := submitOrder(t, client, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_SELL, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 50,
	})
	if err != nil {
		t.Fatalf("SubmitOrder (sell) returned error: %v", err)
	}
	if len(sellResp.Trades) != 0 {
		t.Fatalf("resting sell produced trades: %+v", sellResp.Trades)
	}

	buyResp, err := submitOrder(t, client, &pb.SubmitOrderRequest{
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

func TestServer_SubmitOrderRejectsInvalidQuantity(t *testing.T) {
	client := newTestClient(t)
	registerMarket(t, client, "AAPL", "Apple Inc.")

	_, err := submitOrder(t, client, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 0,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("status code = %v, want InvalidArgument (err: %v)", status.Code(err), err)
	}
}

func TestServer_SubmitOrderFailsForUnregisteredSymbol(t *testing.T) {
	client := newTestClient(t)

	_, err := submitOrder(t, client, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 10,
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("status code = %v, want NotFound (err: %v)", status.Code(err), err)
	}
}

func TestServer_CancelOrder(t *testing.T) {
	client := newTestClient(t)
	registerMarket(t, client, "AAPL", "Apple Inc.")

	submitResp, err := submitOrder(t, client, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_SELL, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 50,
	})
	if err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	cancelResp, err := cancelOrder(t, client, &pb.CancelOrderRequest{
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
	registerMarket(t, client, "AAPL", "Apple Inc.")

	_, err := cancelOrder(t, client, &pb.CancelOrderRequest{
		Symbol: "AAPL", OrderId: 999,
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("status code = %v, want NotFound (err: %v)", status.Code(err), err)
	}
}

func TestServer_CancelOrderFailsForUnregisteredSymbol(t *testing.T) {
	client := newTestClient(t)

	_, err := cancelOrder(t, client, &pb.CancelOrderRequest{
		Symbol: "AAPL", OrderId: 999,
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("status code = %v, want NotFound (err: %v)", status.Code(err), err)
	}
}

func TestServer_GetOrderBook(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()
	registerMarket(t, client, "AAPL", "Apple Inc.")

	if _, err := submitOrder(t, client, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 10,
	}); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}
	if _, err := submitOrder(t, client, &pb.SubmitOrderRequest{
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

func TestServer_GetOrderBookFailsForUnregisteredSymbol(t *testing.T) {
	client := newTestClient(t)

	_, err := client.GetOrderBook(context.Background(), &pb.GetOrderBookRequest{Symbol: "AAPL", Depth: 10})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("status code = %v, want NotFound (err: %v)", status.Code(err), err)
	}
}

func TestServer_ListSymbolsIncludesRegisteredSymbolsEvenBeforeTrading(t *testing.T) {
	client := newTestClient(t)
	registerMarket(t, client, "AAPL", "Apple Inc.")

	resp, err := client.ListSymbols(context.Background(), &pb.ListSymbolsRequest{})
	if err != nil {
		t.Fatalf("ListSymbols returned error: %v", err)
	}
	if len(resp.Markets) != 1 || resp.Markets[0].Symbol != "AAPL" || resp.Markets[0].Description != "Apple Inc." {
		t.Fatalf("markets = %+v, want a single unregistered-but-listed AAPL", resp.Markets)
	}
	if resp.Markets[0].HasTraded {
		t.Fatalf("markets = %+v, want HasTraded=false before any trade", resp.Markets)
	}
}

func TestServer_ListSymbolsReflectsTradingActivity(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()
	registerMarket(t, client, "AAPL", "Apple Inc.")

	if _, err := submitOrder(t, client, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_SELL, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 10,
	}); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}
	if _, err := submitOrder(t, client, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_BUY, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 10,
	}); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	resp, err := client.ListSymbols(ctx, &pb.ListSymbolsRequest{})
	if err != nil {
		t.Fatalf("ListSymbols returned error: %v", err)
	}
	if len(resp.Markets) != 1 || !resp.Markets[0].HasTraded || resp.Markets[0].LastPrice != 100 {
		t.Fatalf("AAPL market = %+v, want HasTraded=true and LastPrice=100", resp.Markets)
	}
}

func TestServer_PingReturnsExchangeEpoch(t *testing.T) {
	client := newTestClient(t)

	resp, err := client.Ping(context.Background(), &pb.PingRequest{})
	if err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
	if resp.Epoch == 0 {
		t.Fatalf("Epoch = 0, want a nonzero random value")
	}
}

func TestServer_SubscribeTradesReceivesLiveTrades(t *testing.T) {
	client := newTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registerMarket(t, client, "AAPL", "Apple Inc.")

	stream, err := client.SubscribeTrades(ctx, &pb.SubscribeTradesRequest{Symbol: "AAPL"})
	if err != nil {
		t.Fatalf("SubscribeTrades returned error: %v", err)
	}
	// Wait for the server to actually register with the engine before submitting, since the
	// broadcast is non-blocking and would silently drop a trade sent before that.
	if _, err := stream.Header(); err != nil {
		t.Fatalf("stream.Header() returned error: %v", err)
	}

	if _, err := submitOrder(t, client, &pb.SubmitOrderRequest{
		Symbol: "AAPL", Side: pb.Side_SELL, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 10,
	}); err != nil {
		t.Fatalf("SubmitOrder (sell) returned error: %v", err)
	}
	if _, err := submitOrder(t, client, &pb.SubmitOrderRequest{
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

func TestServer_SubscribeTradesFailsForUnregisteredSymbol(t *testing.T) {
	client := newTestClient(t)

	stream, err := client.SubscribeTrades(context.Background(), &pb.SubscribeTradesRequest{Symbol: "AAPL"})
	if err != nil {
		t.Fatalf("SubscribeTrades returned error: %v", err)
	}
	if _, err := stream.Recv(); status.Code(err) != codes.NotFound {
		t.Fatalf("status code = %v, want NotFound (err: %v)", status.Code(err), err)
	}
}

// Guards against a real race found live: ListSymbols/RegisterMarket read the best bid/ask via a
// pointer into the engine's own live PriceLevel while trades kept mutating it concurrently.
func TestServer_ListSymbolsIsRaceFreeUnderConcurrentTrading(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()
	registerMarket(t, client, "AAPL", "Apple Inc.")

	const iterations = 200
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			side := pb.Side_BUY
			if i%2 == 1 {
				side = pb.Side_SELL
			}
			_, _ = submitOrder(t, client, &pb.SubmitOrderRequest{
				Symbol: "AAPL", Side: side, Type: pb.OrderType_LIMIT, Price: 100, Quantity: 1,
			})
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if _, err := client.ListSymbols(ctx, &pb.ListSymbolsRequest{}); err != nil {
				t.Errorf("ListSymbols returned error: %v", err)
				return
			}
			if _, err := client.RegisterMarket(ctx, &pb.RegisterMarketRequest{Symbol: "AAPL", Description: "Apple Inc."}); err != nil {
				t.Errorf("RegisterMarket returned error: %v", err)
				return
			}
		}
	}()

	wg.Wait()
}

func TestServer_GetTradeHistoryFailsForUnregisteredSymbol(t *testing.T) {
	client := newTestClient(t)

	_, err := client.GetTradeHistory(context.Background(), &pb.GetTradeHistoryRequest{Symbol: "AAPL"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("status code = %v, want NotFound (err: %v)", status.Code(err), err)
	}
}
