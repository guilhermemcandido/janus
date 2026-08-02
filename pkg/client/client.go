package client

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/vtcodec"
)

// Client is a Go client for the Janus gRPC exchange API.
type Client struct {
	conn *grpc.ClientConn
	stub pb.ExchangeClient
}

// Dial connects to a Janus exchange server at addr, with keepalive pings to detect a dead connection.
func Dial(addr string, opts ...grpc.DialOption) (*Client, error) {
	vtcodec.Register()
	opts = append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                5 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		}),
	}, opts...)
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, stub: pb.NewExchangeClient(conn)}, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// SubmitOrder submits a new order and returns its resulting state plus any trades it caused.
func (c *Client) SubmitOrder(ctx context.Context, symbol string, side Side, typ OrderType, price int64, quantity uint64) (*Order, []Trade, error) {
	resp, err := c.stub.SubmitOrder(ctx, &pb.SubmitOrderRequest{
		Symbol:   symbol,
		Side:     sideToProto(side),
		Type:     typeToProto(typ),
		Price:    price,
		Quantity: quantity,
	})
	if err != nil {
		return nil, nil, err
	}
	return orderFromProto(resp.Order), tradesFromProto(resp.Trades), nil
}

// CancelOrder cancels a resting order by ID and returns its state at the moment of cancellation.
func (c *Client) CancelOrder(ctx context.Context, symbol string, orderID uint64) (*Order, error) {
	resp, err := c.stub.CancelOrder(ctx, &pb.CancelOrderRequest{Symbol: symbol, OrderId: orderID})
	if err != nil {
		return nil, err
	}
	return orderFromProto(resp.Order), nil
}

// GetOrderBook returns an L2 snapshot of up to depth price levels per side.
func (c *Client) GetOrderBook(ctx context.Context, symbol string, depth int) (*BookSnapshot, error) {
	resp, err := c.stub.GetOrderBook(ctx, &pb.GetOrderBookRequest{Symbol: symbol, Depth: int32(depth)})
	if err != nil {
		return nil, err
	}
	return &BookSnapshot{
		Symbol: resp.Symbol,
		Bids:   levelsFromProto(resp.Bids),
		Asks:   levelsFromProto(resp.Asks),
	}, nil
}

// Ping is a lightweight liveness check. Its epoch changes if the exchange process has restarted.
func (c *Client) Ping(ctx context.Context) (epoch uint64, err error) {
	resp, err := c.stub.Ping(ctx, &pb.PingRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Epoch, nil
}

// RegisterMarket lists symbol on the exchange, so it can be traded, watched, and shown as a market.
// Calling it again for an already-registered symbol is a no-op; the first description wins.
func (c *Client) RegisterMarket(ctx context.Context, symbol, description string) (MarketSummary, error) {
	resp, err := c.stub.RegisterMarket(ctx, &pb.RegisterMarketRequest{Symbol: symbol, Description: description})
	if err != nil {
		return MarketSummary{}, err
	}
	return marketSummaryFromProto(resp.Market), nil
}

// ListSymbols returns a market summary for every symbol currently known to the exchange.
func (c *Client) ListSymbols(ctx context.Context) ([]MarketSummary, error) {
	resp, err := c.stub.ListSymbols(ctx, &pb.ListSymbolsRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]MarketSummary, len(resp.Markets))
	for i, m := range resp.Markets {
		out[i] = marketSummaryFromProto(m)
	}
	return out, nil
}

// GetTradeHistory returns the most recent trades for symbol, oldest first.
func (c *Client) GetTradeHistory(ctx context.Context, symbol string) ([]Trade, error) {
	resp, err := c.stub.GetTradeHistory(ctx, &pb.GetTradeHistoryRequest{Symbol: symbol})
	if err != nil {
		return nil, err
	}
	return tradesFromProto(resp.Trades), nil
}
