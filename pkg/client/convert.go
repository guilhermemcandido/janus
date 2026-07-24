package client

import pb "github.com/guilhermemcandido/janus/internal/api/proto"

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
