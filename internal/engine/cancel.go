package engine

import "github.com/guilhermemcandido/janus/internal/types"

// Cancel removes a resting order from the book by ID and returns it, cleaning up its price level if it empties.
func (ob *OrderBook) Cancel(orderID uint64) (*types.Order, error) {
	order, ok := ob.orders[orderID]
	if !ok {
		return nil, ErrOrderNotFound
	}

	side := ob.sideFor(order.Side)
	level, _ := side.Level(order.Price)
	level.Remove(orderID)
	delete(ob.orders, orderID)

	if level.IsEmpty() {
		side.RemoveLevel(order.Price)
	}

	return order, nil
}
