package api

import (
	"context"
	"log"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

func marketNotRegistered(symbol string) error {
	return status.Errorf(codes.NotFound, "market %q is not registered", symbol)
}

func (s *Server) SubmitOrder(ctx context.Context, req *pb.SubmitOrderRequest) (*pb.SubmitOrderResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}

	eng, ok := s.exchange.Lookup(req.Symbol)
	if !ok {
		return nil, marketNotRegistered(req.Symbol)
	}
	order := types.NewOrder(req.Symbol, sideFromProto(req.Side), typeFromProto(req.Type), req.Price, req.Quantity)

	trades, err := eng.Submit(order)
	if err != nil {
		return nil, toStatus(err)
	}
	for _, tr := range trades {
		log.Printf("trade: %s %d @ %d (maker %d, taker %d)", req.Symbol, tr.Quantity, tr.Price, tr.MakerOrderID, tr.TakerOrderID)
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

	eng, ok := s.exchange.Lookup(req.Symbol)
	if !ok {
		return nil, marketNotRegistered(req.Symbol)
	}

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

	eng, ok := s.exchange.Lookup(req.Symbol)
	if !ok {
		return nil, marketNotRegistered(req.Symbol)
	}
	snap := eng.Depth(int(req.Depth))

	return &pb.GetOrderBookResponse{
		Symbol: snap.Symbol,
		Bids:   levelsToProto(snap.Bids),
		Asks:   levelsToProto(snap.Asks),
	}, nil
}

// Ping's Epoch changes when the exchange restarts, so clients can detect a lost state.
func (s *Server) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	return &pb.PingResponse{Epoch: s.exchange.Epoch}, nil
}

// ListSymbols returns a market summary for every registered symbol, sorted by symbol.
func (s *Server) ListSymbols(ctx context.Context, req *pb.ListSymbolsRequest) (*pb.ListSymbolsResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}

	symbols := s.exchange.Symbols()
	sort.Strings(symbols)

	markets := make([]*pb.MarketSummary, len(symbols))
	for i, symbol := range symbols {
		eng, _ := s.exchange.Lookup(symbol) // came from Symbols(), so it's guaranteed to be registered
		markets[i] = marketSummaryToProto(s.exchange.Description(symbol), eng.Stats(), eng.BestBid(), eng.BestAsk())
	}

	return &pb.ListSymbolsResponse{Markets: markets}, nil
}

// GetTradeHistory returns the most recent trades for a symbol, oldest first.
func (s *Server) GetTradeHistory(ctx context.Context, req *pb.GetTradeHistoryRequest) (*pb.GetTradeHistoryResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}

	eng, ok := s.exchange.Lookup(req.Symbol)
	if !ok {
		return nil, marketNotRegistered(req.Symbol)
	}
	trades := eng.TradeHistory()

	return &pb.GetTradeHistoryResponse{Symbol: req.Symbol, Trades: tradesToProto(req.Symbol, trades)}, nil
}

// RegisterMarket lists symbol on the exchange, so it can be traded, watched, and shown as a market.
// The symbol is upper-cased so a market's identity is never sensitive to how it was typed -
// otherwise a market's own card could link to a symbol nothing was ever registered under.
func (s *Server) RegisterMarket(ctx context.Context, req *pb.RegisterMarketRequest) (*pb.RegisterMarketResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if symbol == "" {
		return nil, status.Error(codes.InvalidArgument, "symbol is required")
	}

	eng := s.exchange.Register(symbol, req.Description)
	market := marketSummaryToProto(s.exchange.Description(symbol), eng.Stats(), eng.BestBid(), eng.BestAsk())
	return &pb.RegisterMarketResponse{Market: market}, nil
}

func (s *Server) SubscribeTrades(req *pb.SubscribeTradesRequest, stream pb.Exchange_SubscribeTradesServer) error {
	eng, ok := s.exchange.Lookup(req.Symbol)
	if !ok {
		return marketNotRegistered(req.Symbol)
	}
	trades, unsub := eng.Subscribe()
	defer unsub()

	// Signals registration is done, closing the race with the non-blocking broadcast.
	if err := stream.SendHeader(metadata.MD{}); err != nil {
		return err
	}

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
