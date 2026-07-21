package engine

import "github.com/guilhermemcandido/janus/internal/types"

// Submit matches order by price-time priority; unfilled limit quantity rests, market orders never do.
func (ob *OrderBook) Submit(order *types.Order) []types.Trade {
	order.Remaining = order.Quantity
	order.Timestamp = int64(ob.nextSeq())

	opposite := ob.opposite(order.Side)
	var trades []types.Trade

	for order.Remaining > 0 {
		level := opposite.Best()
		if level == nil {
			break
		}
		if order.Type == types.Limit && !crosses(order, level.Price) {
			break
		}
		trades = append(trades, ob.matchLevel(order, level)...)
		if level.IsEmpty() {
			opposite.RemoveLevel(level.Price)
		}
	}

	if order.Remaining > 0 && order.Type == types.Limit {
		ob.rest(order)
	}

	return trades
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

		seq := ob.nextSeq()
		trades = append(trades, types.Trade{
			ID:           seq,
			Timestamp:    int64(seq),
			Price:        level.Price,
			Quantity:     qty,
			MakerOrderID: maker.ID,
			TakerOrderID: taker.ID,
		})

		taker.Remaining -= qty
		maker.Remaining -= qty

		if maker.Remaining == 0 {
			level.PopFront()
			delete(ob.Orders, maker.ID)
		}
	}
	return trades
}

// rest adds order to its own side of the book, tracked by both the price level and the order index.
func (ob *OrderBook) rest(order *types.Order) {
	ob.sideFor(order.Side).GetOrCreateLevel(order.Price).Add(order)
	ob.Orders[order.ID] = order
}

// sideFor returns the side an order rests on.
func (ob *OrderBook) sideFor(side types.Side) *BookSide {
	if side == types.Buy {
		return ob.Bids
	}
	return ob.Asks
}

// opposite returns the side an order matches against.
func (ob *OrderBook) opposite(side types.Side) *BookSide {
	if side == types.Buy {
		return ob.Asks
	}
	return ob.Bids
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
