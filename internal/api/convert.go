package api

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

// toStatus maps a domain error to the gRPC status code a client should see.
func toStatus(err error) error {
	switch err {
	case engine.ErrInvalidQuantity, engine.ErrInvalidPrice, engine.ErrSymbolMismatch:
		return status.Error(codes.InvalidArgument, err.Error())
	case engine.ErrOrderNotFound:
		return status.Error(codes.NotFound, err.Error())
	case engine.ErrEngineStopped:
		return status.Error(codes.Unavailable, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
