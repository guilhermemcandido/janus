package api

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/engine"
	"github.com/guilhermemcandido/janus/internal/types"
)

// Server implements the janus.Exchange gRPC service, routing each request to the right Engine by symbol.
type Server struct {
	pb.UnimplementedExchangeServer
	exchange *engine.Exchange
}

func NewServer(exchange *engine.Exchange) *Server {
	return &Server{exchange: exchange}
}

func (s *Server) SubmitOrder(ctx context.Context, req *pb.SubmitOrderRequest) (*pb.SubmitOrderResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}

	eng := s.exchange.GetOrCreateEngine(req.Symbol)
	order := types.NewOrder(req.Symbol, sideFromProto(req.Side), typeFromProto(req.Type), req.Price, req.Quantity)

	trades, err := eng.Submit(order)
	if err != nil {
		return nil, toStatus(err)
	}

	return &pb.SubmitOrderResponse{
		Order:  orderToProto(order),
		Trades: tradesToProto(req.Symbol, trades),
	}, nil
}

func (s *Server) CancelOrder(ctx context.Context, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}

	eng := s.exchange.GetOrCreateEngine(req.Symbol)

	order, err := eng.Cancel(req.OrderId)
	if err != nil {
		return nil, toStatus(err)
	}

	return &pb.CancelOrderResponse{Order: orderToProto(order)}, nil
}

func (s *Server) GetOrderBook(ctx context.Context, req *pb.GetOrderBookRequest) (*pb.GetOrderBookResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if req.Depth < 0 {
		return nil, status.Error(codes.InvalidArgument, "depth must not be negative")
	}

	eng := s.exchange.GetOrCreateEngine(req.Symbol)
	snap := eng.Depth(int(req.Depth))

	return &pb.GetOrderBookResponse{
		Symbol: snap.Symbol,
		Bids:   levelsToProto(snap.Bids),
		Asks:   levelsToProto(snap.Asks),
	}, nil
}

func (s *Server) SubscribeTrades(req *pb.SubscribeTradesRequest, stream pb.Exchange_SubscribeTradesServer) error {
	eng := s.exchange.GetOrCreateEngine(req.Symbol)
	trades, unsub := eng.Subscribe()
	defer unsub()

	for {
		select {
		case tr, ok := <-trades:
			if !ok {
				return nil
			}
			if err := stream.Send(tradeToProto(req.Symbol, tr)); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}
