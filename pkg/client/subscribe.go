package client

import (
	"context"
	"time"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
)

const (
	reconnectInitialBackoff = 250 * time.Millisecond
	reconnectMaxBackoff     = 5 * time.Second
	pingTimeout             = 2 * time.Second
)

// SubscribeTrades returns a channel of live trades for symbol, reconnecting automatically if the stream breaks.
func (c *Client) SubscribeTrades(ctx context.Context, symbol string) (<-chan Trade, error) {
	stream, err := c.subscribeAndWaitReady(ctx, symbol)
	if err != nil {
		return nil, err
	}

	out := make(chan Trade)
	go func() {
		defer close(out)
		for {
			tr, recvErr := stream.Recv()
			if recvErr != nil {
				if ctx.Err() != nil {
					return
				}
				var reconnectErr error
				stream, reconnectErr = c.reconnectSubscribeTrades(ctx, symbol)
				if reconnectErr != nil {
					return
				}
				continue
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

// reconnectSubscribeTrades waits for Ping to succeed before resubscribing, backing off between attempts.
func (c *Client) reconnectSubscribeTrades(ctx context.Context, symbol string) (pb.Exchange_SubscribeTradesClient, error) {
	backoff := reconnectInitialBackoff
	for {
		pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
		_, pingErr := c.Ping(pingCtx)
		cancel()

		if pingErr == nil {
			if stream, err := c.subscribeAndWaitReady(ctx, symbol); err == nil {
				return stream, nil
			}
		}

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		backoff *= 2
		if backoff > reconnectMaxBackoff {
			backoff = reconnectMaxBackoff
		}
	}
}

// subscribeAndWaitReady waits for headers, sent only once registered, so a trade can't be dropped.
func (c *Client) subscribeAndWaitReady(ctx context.Context, symbol string) (pb.Exchange_SubscribeTradesClient, error) {
	stream, err := c.stub.SubscribeTrades(ctx, &pb.SubscribeTradesRequest{Symbol: symbol})
	if err != nil {
		return nil, err
	}
	if _, err := stream.Header(); err != nil {
		return nil, err
	}
	return stream, nil
}
