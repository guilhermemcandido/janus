package client

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
)

// errClientClosed is returned to any command still waiting when Close is called.
var errClientClosed = errors.New("client closed")

// orderStream shares one OrderStream RPC across every Submit/Cancel call from this Client, instead
// of opening a fresh gRPC stream per order. Its lifetime is the Client's, not any single call's ctx.
type orderStream struct {
	c        *Client
	closeCtx context.Context
	closeFn  context.CancelFunc

	mu      sync.Mutex
	stream  pb.Exchange_OrderStreamClient
	pending map[uint64]chan *pb.OrderEvent
	lastErr error

	nextID   atomic.Uint64
	initOnce sync.Once
}

func newOrderStream(c *Client) *orderStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &orderStream{
		c:        c,
		closeCtx: ctx,
		closeFn:  cancel,
		pending:  make(map[uint64]chan *pb.OrderEvent),
	}
}

// send stamps cmd with a fresh correlation ID, writes it to the shared stream, and waits for the
// matching reply - or for ctx or the client itself to end first.
func (s *orderStream) send(ctx context.Context, cmd *pb.OrderCommand) (*pb.OrderEvent, error) {
	s.initOnce.Do(s.start)

	id := s.nextID.Add(1)
	cmd.CorrelationId = id
	reply := make(chan *pb.OrderEvent, 1)

	s.mu.Lock()
	if s.lastErr != nil {
		err := s.lastErr
		s.mu.Unlock()
		return nil, err
	}
	s.pending[id] = reply
	err := s.stream.Send(cmd)
	s.mu.Unlock()
	if err != nil {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, err
	}

	select {
	case evt := <-reply:
		return evt, nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, ctx.Err()
	case <-s.closeCtx.Done():
		return nil, errClientClosed
	}
}

// start dials the initial stream (if it fails, run's first iteration reconnects) and launches the
// goroutine that reads replies and reconnects for as long as the Client stays open.
func (s *orderStream) start() {
	stream, err := s.c.stub.OrderStream(s.closeCtx)
	s.mu.Lock()
	if err != nil {
		s.lastErr = err
	} else {
		s.stream = stream
	}
	s.mu.Unlock()
	go s.run()
}

func (s *orderStream) run() {
	for {
		s.mu.Lock()
		stream := s.stream
		s.mu.Unlock()
		if stream != nil {
			s.readLoop(stream)
		}
		if s.closeCtx.Err() != nil {
			return
		}
		stream, err := s.reconnect()
		if err != nil {
			return // only returns non-nil when closeCtx is done
		}
		s.mu.Lock()
		s.stream = stream
		s.lastErr = nil
		s.mu.Unlock()
	}
}

// readLoop dispatches replies by correlation ID until stream breaks, then fails every caller still
// waiting on it so they don't hang until ctx expires on its own.
func (s *orderStream) readLoop(stream pb.Exchange_OrderStreamClient) {
	for {
		evt, err := stream.Recv()
		if err != nil {
			s.failAllPending(err)
			return
		}
		s.mu.Lock()
		reply, ok := s.pending[evt.CorrelationId]
		delete(s.pending, evt.CorrelationId)
		s.mu.Unlock()
		if ok {
			reply <- evt
		}
	}
}

func (s *orderStream) failAllPending(err error) {
	s.mu.Lock()
	s.lastErr = err
	pending := s.pending
	s.pending = make(map[uint64]chan *pb.OrderEvent)
	s.mu.Unlock()

	evt := &pb.OrderEvent{Event: &pb.OrderEvent_Error{Error: &pb.OrderError{
		Code: uint32(codes.Unavailable), Message: err.Error(),
	}}}
	for _, reply := range pending {
		reply <- evt
	}
}

// reconnect waits for Ping to succeed before reopening the stream, backing off between attempts -
// the same pattern SubscribeTrades uses.
func (s *orderStream) reconnect() (pb.Exchange_OrderStreamClient, error) {
	backoff := reconnectInitialBackoff
	for {
		pingCtx, cancel := context.WithTimeout(s.closeCtx, pingTimeout)
		_, pingErr := s.c.Ping(pingCtx)
		cancel()
		if pingErr == nil {
			if stream, err := s.c.stub.OrderStream(s.closeCtx); err == nil {
				return stream, nil
			}
		}

		select {
		case <-time.After(backoff):
		case <-s.closeCtx.Done():
			return nil, s.closeCtx.Err()
		}
		backoff *= 2
		if backoff > reconnectMaxBackoff {
			backoff = reconnectMaxBackoff
		}
	}
}
