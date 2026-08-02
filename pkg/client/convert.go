package client

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
)

// orderEventErr reconstructs a normal status error from an in-band OrderEvent_Error, so callers can
// still use status.Code(err) exactly as they could with the old unary RPCs.
func orderEventErr(e *pb.OrderError) error {
	return status.Error(codes.Code(e.Code), e.Message)
}

func sideToProto(s Side) pb.Side {
	if s == Sell {
		return pb.Side_SELL
	}
	return pb.Side_BUY
}

func typeToProto(t OrderType) pb.OrderType {
	if t == Market {
		return pb.OrderType_MARKET
	}
	return pb.OrderType_LIMIT
}

func sideFromProto(s pb.Side) Side {
	if s == pb.Side_SELL {
		return Sell
	}
	return Buy
}

func typeFromProto(t pb.OrderType) OrderType {
	if t == pb.OrderType_MARKET {
		return Market
	}
	return Limit
}

func orderFromProto(o *pb.Order) *Order {
	if o == nil {
		return nil
	}
	return &Order{
		ID:        o.Id,
		Symbol:    o.Symbol,
		Side:      sideFromProto(o.Side),
		Type:      typeFromProto(o.Type),
		Price:     o.Price,
		Quantity:  o.Quantity,
		Remaining: o.Remaining,
	}
}

func tradeFromProto(tr *pb.Trade) Trade {
	return Trade{
		ID:           tr.Id,
		Symbol:       tr.Symbol,
		Price:        tr.Price,
		Quantity:     tr.Quantity,
		MakerOrderID: tr.MakerOrderId,
		TakerOrderID: tr.TakerOrderId,
	}
}

func tradesFromProto(trades []*pb.Trade) []Trade {
	out := make([]Trade, len(trades))
	for i, tr := range trades {
		out[i] = tradeFromProto(tr)
	}
	return out
}

func levelsFromProto(levels []*pb.PriceLevel) []PriceLevel {
	out := make([]PriceLevel, len(levels))
	for i, l := range levels {
		out[i] = PriceLevel{Price: l.Price, Quantity: l.Quantity}
	}
	return out
}

// levelFromProto converts a top-of-book level, or nil if that side of the book is empty.
func levelFromProto(l *pb.PriceLevel) *PriceLevel {
	if l == nil {
		return nil
	}
	return &PriceLevel{Price: l.Price, Quantity: l.Quantity}
}

func marketSummaryFromProto(m *pb.MarketSummary) MarketSummary {
	return MarketSummary{
		Symbol:      m.Symbol,
		Description: m.Description,
		HasTraded:   m.HasTraded,
		LastPrice:   m.LastPrice,
		OpenPrice:   m.OpenPrice,
		High:        m.High,
		Low:         m.Low,
		Volume:      m.Volume,
		BestBid:     levelFromProto(m.BestBid),
		BestAsk:     levelFromProto(m.BestAsk),
	}
}
