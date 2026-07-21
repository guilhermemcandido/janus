package engine

import (
	"container/list"

	"github.com/guilhermemcandido/janus/internal/types"
)

// PriceLevel holds all resting orders at a single price, FIFO by arrival.
type PriceLevel struct {
	price  int64
	orders *list.List
	index  map[uint64]*list.Element // orderID -> element, for O(1) cancel
}

func NewPriceLevel(price int64) *PriceLevel {
	return &PriceLevel{
		price:  price,
		orders: list.New(),
		index:  make(map[uint64]*list.Element),
	}
}

func (pl *PriceLevel) Price() int64 {
	return pl.price
}

// Add appends an order to the back of the queue.
func (pl *PriceLevel) Add(o *types.Order) {
	el := pl.orders.PushBack(o)
	pl.index[o.ID] = el
}

// Remove cancels an order in O(1); reports whether it was present.
func (pl *PriceLevel) Remove(orderID uint64) bool {
	el, ok := pl.index[orderID]
	if !ok {
		return false
	}
	pl.orders.Remove(el)
	delete(pl.index, orderID)
	return true
}

// Front returns the oldest order at this level, or nil if empty.
func (pl *PriceLevel) Front() *types.Order {
	el := pl.orders.Front()
	if el == nil {
		return nil
	}
	return el.Value.(*types.Order)
}

// PopFront removes and returns the oldest order, or nil if empty.
func (pl *PriceLevel) PopFront() *types.Order {
	el := pl.orders.Front()
	if el == nil {
		return nil
	}
	pl.orders.Remove(el)
	o := el.Value.(*types.Order)
	delete(pl.index, o.ID)
	return o
}

func (pl *PriceLevel) Len() int {
	return pl.orders.Len()
}

func (pl *PriceLevel) IsEmpty() bool {
	return pl.orders.Len() == 0
}
