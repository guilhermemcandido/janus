package futures

import (
	"context"

	"github.com/guilhermemcandido/janus/pkg/client"
)

// SpotMidPriceSource prices a futures instrument as the spot instrument's mid-price plus a fixed basis.
// This is a simple cost-of-carry-style model, not a full quant pricing curve.
type SpotMidPriceSource struct {
	c      *client.Client
	symbol string
	basis  int64
	last   int64 // last price returned; used whenever a fresh mid isn't available
}

// NewSpotMidPriceSource tracks spotSymbol's mid-price, offset by basis. initial seeds the price
// returned before the spot book has ever had both a bid and an ask to compute a mid from.
func NewSpotMidPriceSource(c *client.Client, spotSymbol string, basis, initial int64) *SpotMidPriceSource {
	return &SpotMidPriceSource{c: c, symbol: spotSymbol, basis: basis, last: initial}
}

// Price implements bots.PriceSource, reusing the last known-good price if the spot book momentarily lacks a bid or ask.
func (s *SpotMidPriceSource) Price(ctx context.Context) (int64, error) {
	book, err := s.c.GetOrderBook(ctx, s.symbol, 1)
	if err != nil {
		return 0, err
	}
	if len(book.Bids) == 0 || len(book.Asks) == 0 {
		return s.last, nil
	}
	mid := (book.Bids[0].Price + book.Asks[0].Price) / 2
	s.last = mid + s.basis
	return s.last, nil
}
