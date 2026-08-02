package engine

import "github.com/guilhermemcandido/janus/internal/types"

// maxFreeNodes bounds how many emptied orderNodes a PriceLevel holds onto for reuse.
const maxFreeNodes = 256

// orderNode is one node in PriceLevel's intrusive FIFO queue, replacing container/list.Element so
// emptied nodes can be pooled instead of always allocating a fresh one.
type orderNode struct {
	order      *types.Order
	next, prev *orderNode
}

// PriceLevel holds all resting orders at a single price, FIFO by arrival.
type PriceLevel struct {
	price      int64
	head, tail *orderNode
	length     int
	index      map[uint64]*orderNode // orderID -> node, for O(1) cancel
	free       []*orderNode
}

func NewPriceLevel(price int64) *PriceLevel {
	return &PriceLevel{
		price: price,
		index: make(map[uint64]*orderNode),
	}
}

func (pl *PriceLevel) Price() int64 {
	return pl.price
}

// reset reinitializes pl for reuse at a new price, keeping its index map and node freelist instead
// of allocating fresh ones - see BookSide's own freelist.
func (pl *PriceLevel) reset(price int64) {
	pl.price = price
	pl.head, pl.tail = nil, nil
	pl.length = 0
	clear(pl.index)
}

// newNode reuses a freed node if one's available, avoiding an allocation for what's typically a
// level being refilled after a fill.
func (pl *PriceLevel) newNode(o *types.Order) *orderNode {
	if n := len(pl.free); n > 0 {
		node := pl.free[n-1]
		pl.free = pl.free[:n-1]
		node.order, node.next, node.prev = o, nil, nil
		return node
	}
	return &orderNode{order: o}
}

// release unlinks node from the list, drops its order reference, and returns it to the freelist.
func (pl *PriceLevel) release(node *orderNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		pl.head = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		pl.tail = node.prev
	}
	pl.length--

	node.order = nil
	if len(pl.free) < maxFreeNodes {
		pl.free = append(pl.free, node)
	}
}

// Add appends an order to the back of the queue.
func (pl *PriceLevel) Add(o *types.Order) {
	node := pl.newNode(o)
	if pl.tail == nil {
		pl.head, pl.tail = node, node
	} else {
		node.prev = pl.tail
		pl.tail.next = node
		pl.tail = node
	}
	pl.length++
	pl.index[o.ID] = node
}

// Remove cancels an order in O(1); reports whether it was present.
func (pl *PriceLevel) Remove(orderID uint64) bool {
	node, ok := pl.index[orderID]
	if !ok {
		return false
	}
	delete(pl.index, orderID)
	pl.release(node)
	return true
}

// Front returns the oldest order at this level, or nil if empty.
func (pl *PriceLevel) Front() *types.Order {
	if pl.head == nil {
		return nil
	}
	return pl.head.order
}

// PopFront removes and returns the oldest order, or nil if empty.
func (pl *PriceLevel) PopFront() *types.Order {
	node := pl.head
	if node == nil {
		return nil
	}
	o := node.order
	delete(pl.index, o.ID)
	pl.release(node)
	return o
}

func (pl *PriceLevel) Len() int {
	return pl.length
}

func (pl *PriceLevel) IsEmpty() bool {
	return pl.length == 0
}

// Orders returns every order at this level, oldest first.
func (pl *PriceLevel) Orders() []*types.Order {
	out := make([]*types.Order, 0, pl.length)
	for node := pl.head; node != nil; node = node.next {
		out = append(out, node.order)
	}
	return out
}

// TotalQuantity sums Remaining across every order resting at this level.
func (pl *PriceLevel) TotalQuantity() uint64 {
	var total uint64
	for node := pl.head; node != nil; node = node.next {
		total += node.order.Remaining
	}
	return total
}
