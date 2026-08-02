package api

import (
	"google.golang.org/grpc/codes"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/engine"
	"github.com/guilhermemcandido/janus/internal/types"
)

func sideFromProto(s pb.Side) types.Side {
	if s == pb.Side_SELL {
		return types.Sell
	}
	return types.Buy
}

func typeFromProto(t pb.OrderType) types.OrderType {
	if t == pb.OrderType_MARKET {
		return types.Market
	}
	return types.Limit
}

func sideToProto(s types.Side) pb.Side {
	if s == types.Sell {
		return pb.Side_SELL
	}
	return pb.Side_BUY
}

func typeToProto(t types.OrderType) pb.OrderType {
	if t == types.Market {
		return pb.OrderType_MARKET
	}
	return pb.OrderType_LIMIT
}

func orderToProto(o *types.Order) *pb.Order {
	return &pb.Order{
		Id:        o.ID,
		Symbol:    o.Symbol,
		Side:      sideToProto(o.Side),
		Type:      typeToProto(o.Type),
		Price:     o.Price,
		Quantity:  o.Quantity,
		Remaining: o.Remaining,
	}
}

func tradeToProto(symbol string, tr types.Trade) *pb.Trade {
	return &pb.Trade{
		Id:           tr.ID,
		Symbol:       symbol,
		Price:        tr.Price,
		Quantity:     tr.Quantity,
		MakerOrderId: tr.MakerOrderID,
		TakerOrderId: tr.TakerOrderID,
	}
}

func tradesToProto(symbol string, trades []types.Trade) []*pb.Trade {
	out := make([]*pb.Trade, len(trades))
	for i, tr := range trades {
		out[i] = tradeToProto(symbol, tr)
	}
	return out
}

func levelsToProto(levels []types.PriceLevelSnapshot) []*pb.PriceLevel {
	out := make([]*pb.PriceLevel, len(levels))
	for i, l := range levels {
		out[i] = &pb.PriceLevel{Price: l.Price, Quantity: l.Quantity}
	}
	return out
}

// bestLevelToProto converts a top-of-book snapshot, or nil if that side of the book is empty.
func bestLevelToProto(l *types.PriceLevelSnapshot) *pb.PriceLevel {
	if l == nil {
		return nil
	}
	return &pb.PriceLevel{Price: l.Price, Quantity: l.Quantity}
}

func marketSummaryToProto(description string, stats types.MarketStats, bestBid, bestAsk *types.PriceLevelSnapshot) *pb.MarketSummary {
	return &pb.MarketSummary{
		Symbol:      stats.Symbol,
		Description: description,
		HasTraded:   stats.HasTraded,
		LastPrice:   stats.LastPrice,
		OpenPrice:   stats.OpenPrice,
		High:        stats.High,
		Low:         stats.Low,
		Volume:      stats.Volume,
		BestBid:     bestLevelToProto(bestBid),
		BestAsk:     bestLevelToProto(bestAsk),
	}
}

// toCode maps a domain error to the gRPC status code a client should see.
func toCode(err error) codes.Code {
	switch err {
	case engine.ErrInvalidQuantity, engine.ErrInvalidPrice, engine.ErrPriceOutOfRange, engine.ErrSymbolMismatch:
		return codes.InvalidArgument
	case engine.ErrOrderNotFound:
		return codes.NotFound
	case engine.ErrEngineStopped:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}
