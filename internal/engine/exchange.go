package engine

// Exchange holds one OrderBook per traded symbol, creating each on first use.
type Exchange struct {
	books map[string]*OrderBook
}

func NewExchange() *Exchange {
	return &Exchange{books: make(map[string]*OrderBook)}
}

// Book returns the OrderBook for symbol, creating it the first time it's requested.
func (e *Exchange) Book(symbol string) *OrderBook {
	if b, ok := e.books[symbol]; ok {
		return b
	}
	b := NewOrderBook(symbol)
	e.books[symbol] = b
	return b
}
