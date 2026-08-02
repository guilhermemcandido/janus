package engine

import "github.com/guilhermemcandido/janus/internal/types"

// Submit matches order by price-time priority; unfilled limit quantity rests, market orders never do.
func (ob *OrderBook) Submit(order *types.Order) ([]types.Trade, error) {
	if order.Symbol != ob.Symbol {
		return nil, ErrSymbolMismatch
	}
	if order.Quantity == 0 {
		return nil, ErrInvalidQuantity
	}
	if order.Type == types.Limit && order.Price <= 0 {
		return nil, ErrInvalidPrice
	}
	// Checked upfront, even though a marketable order this far out would never actually need to
	// rest, so a rejection never happens after trades have already executed.
	if order.Type == types.Limit && order.Price > maxTickPrice {
		return nil, ErrPriceOutOfRange
	}

	order.ID = ob.nextSeq()

	opposite := ob.opposite(order.Side)
	var trades []types.Trade

	for order.Remaining > 0 {
		level := opposite.Best()
		if level == nil {
			break
		}
		if order.Type == types.Limit && !crosses(order, level.Price()) {
			break
		}
		trades = append(trades, ob.matchLevel(order, level)...)
		if level.IsEmpty() {
			opposite.RemoveLevel(level.Price())
		}
	}

	if order.Remaining > 0 && order.Type == types.Limit {
		ob.rest(order)
	}

	ob.recordTrades(trades)
	return trades, nil
}

// recordTrades updates market stats and the trade history ring for a batch of newly matched trades.
func (ob *OrderBook) recordTrades(trades []types.Trade) {
	for _, tr := range trades {
		if !ob.stats.HasTraded {
			ob.stats.HasTraded = true
			ob.stats.OpenPrice = tr.Price
			ob.stats.High = tr.Price
			ob.stats.Low = tr.Price
		}
		ob.stats.LastPrice = tr.Price
		if tr.Price > ob.stats.High {
			ob.stats.High = tr.Price
		}
		if tr.Price < ob.stats.Low {
			ob.stats.Low = tr.Price
		}
		ob.stats.Volume += tr.Quantity
		ob.history.add(tr)
	}
}

// matchLevel fills taker against level's FIFO queue until either is exhausted.
func (ob *OrderBook) matchLevel(taker *types.Order, level *PriceLevel) []types.Trade {
	var trades []types.Trade
	for taker.Remaining > 0 && !level.IsEmpty() {
		maker := level.Front()
		qty := taker.Remaining
		if maker.Remaining < qty {
			qty = maker.Remaining
		}

		trades = append(trades, types.Trade{
			ID:           ob.nextSeq(),
			Price:        level.Price(),
			Quantity:     qty,
			MakerOrderID: maker.ID,
			TakerOrderID: taker.ID,
		})

		taker.Remaining -= qty
		maker.Remaining -= qty

		if maker.Remaining == 0 {
			level.PopFront()
			delete(ob.orders, maker.ID)
		}
	}
	return trades
}

// rest stores a copy of order, since the caller keeps its own pointer after Submit returns.
func (ob *OrderBook) rest(order *types.Order) {
	resting := *order
	ob.sideFor(order.Side).GetOrCreateLevel(order.Price).Add(&resting)
	ob.orders[order.ID] = &resting
}

// sideFor returns the side an order rests on.
func (ob *OrderBook) sideFor(side types.Side) *BookSide {
	if side == types.Buy {
		return ob.bids
	}
	return ob.asks
}

// opposite returns the side an order matches against.
func (ob *OrderBook) opposite(side types.Side) *BookSide {
	if side == types.Buy {
		return ob.asks
	}
	return ob.bids
}

func (ob *OrderBook) nextSeq() uint64 {
	ob.seq++
	return ob.seq
}

// crosses reports whether order's limit price would accept a match at oppositePrice.
func crosses(order *types.Order, oppositePrice int64) bool {
	if order.Side == types.Buy {
		return order.Price >= oppositePrice
	}
	return order.Price <= oppositePrice
}
