package engine

import "github.com/guilhermemcandido/janus/internal/types"

type subscription struct {
	id uint64
	ch chan types.Trade
}

type subscribeCommand struct {
	reply chan subscription
}

type unsubscribeCommand struct {
	subID uint64
	reply chan struct{}
}

// subscribers is a registry of live trade subscriptions, keyed by an ID unique for the Engine's lifetime.
type subscribers struct {
	byID map[uint64]chan types.Trade
	next uint64
}

func newSubscribers() *subscribers {
	return &subscribers{byID: make(map[uint64]chan types.Trade)}
}

// add registers ch and returns its new subscription ID.
func (s *subscribers) add(ch chan types.Trade) uint64 {
	s.next++
	s.byID[s.next] = ch
	return s.next
}

// remove closes and deregisters id, if it exists.
func (s *subscribers) remove(id uint64) {
	if ch, ok := s.byID[id]; ok {
		delete(s.byID, id)
		close(ch)
	}
}

// closeAll closes every remaining subscription, for use during Engine shutdown.
func (s *subscribers) closeAll() {
	for _, ch := range s.byID {
		close(ch)
	}
}

// broadcast sends tr to every subscriber, dropping it for anyone too slow to keep up rather than blocking matching.
func (s *subscribers) broadcast(tr types.Trade) {
	for _, ch := range s.byID {
		select {
		case ch <- tr:
		default:
		}
	}
}

// Subscribe returns a channel of live trades and an unsubscribe function to stop receiving and release it.
func (e *Engine) Subscribe() (<-chan types.Trade, func()) {
	reply := make(chan subscription, 1)
	sub, err := call(e, subscribeCommand{reply: reply}, reply)
	if err != nil {
		ch := make(chan types.Trade)
		close(ch)
		return ch, func() {}
	}
	return sub.ch, func() {
		reply := make(chan struct{}, 1)
		call(e, unsubscribeCommand{subID: sub.id, reply: reply}, reply)
	}
}

func (e *Engine) broadcast(trades []types.Trade) {
	for _, tr := range trades {
		e.subs.broadcast(tr)
	}
}
