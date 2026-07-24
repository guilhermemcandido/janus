package client

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
)

// Client is a Go client for the Janus gRPC exchange API.
type Client struct {
	conn *grpc.ClientConn
	stub pb.ExchangeClient
}

// Dial connects to a Janus exchange server at addr.
func Dial(addr string, opts ...grpc.DialOption) (*Client, error) {
	opts = append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, opts...)
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

// SubscribeTrades returns a channel of live trades for symbol; it closes once ctx is cancelled or the server ends the stream.
func (c *Client) SubscribeTrades(ctx context.Context, symbol string) (<-chan Trade, error) {
	stream, err := c.stub.SubscribeTrades(ctx, &pb.SubscribeTradesRequest{Symbol: symbol})
	if err != nil {
		return nil, err
	}

	out := make(chan Trade)
	go func() {
		defer close(out)
		for {
			tr, err := stream.Recv()
			if err != nil {
				return
			}
			select {
			case out <- tradeFromProto(tr):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
