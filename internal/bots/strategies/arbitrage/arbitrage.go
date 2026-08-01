package arbitrage

import (
	"context"
	"fmt"
	"io"

	"github.com/guilhermemcandido/janus/pkg/client"
)

// Arbitrage trades the spot/futures spread against a fixed fair basis, closing once it reverts.
// Doesn't reconcile leg-by-leg fill mismatches; both legs are sent for the same quantity.
type Arbitrage struct {
	c          *client.Client
	cfg        Config
	long       bool // true if currently long the basis (long futures, short spot); only meaningful if inPosition
	inPosition bool

	lastFailed bool
}

// New creates an Arbitrage strategy for cfg, trading through c.
func New(c *client.Client, cfg Config) *Arbitrage {
	return &Arbitrage{c: c, cfg: cfg}
}

// Act implements bots.Strategy.
func (a *Arbitrage) Act(ctx context.Context, out io.Writer) error {
	a.lastFailed = false
	spotMid, ok := a.mid(ctx, a.cfg.SpotSymbol)
	if !ok {
		return nil
	}
	futuresMid, ok := a.mid(ctx, a.cfg.FuturesSymbol)
	if !ok {
		return nil
	}
	deviation := (futuresMid - spotMid) - a.cfg.FairBasis

	if !a.inPosition {
		a.tryEnter(ctx, out, deviation)
	} else {
		a.tryExit(ctx, out, deviation)
	}
	return nil
}

func (a *Arbitrage) tryEnter(ctx context.Context, out io.Writer, deviation int64) {
	switch {
	case deviation > a.cfg.EntryThreshold:
		// futures rich relative to fair value: sell futures, buy spot, expecting the spread to fall.
		a.trade(ctx, out, client.Sell, client.Buy)
		a.long, a.inPosition = false, true
		fmt.Fprintf(out, "arb: entered short-basis (deviation %d)\n", deviation)
	case deviation < -a.cfg.EntryThreshold:
		// futures cheap relative to fair value: buy futures, sell spot, expecting the spread to rise.
		a.trade(ctx, out, client.Buy, client.Sell)
		a.long, a.inPosition = true, true
		fmt.Fprintf(out, "arb: entered long-basis (deviation %d)\n", deviation)
	}
}

func (a *Arbitrage) tryExit(ctx context.Context, out io.Writer, deviation int64) {
	exitBand := a.cfg.EntryThreshold / 2
	reverted := deviation >= -exitBand && deviation <= exitBand
	if !reverted {
		return
	}
	if a.long {
		a.trade(ctx, out, client.Sell, client.Buy) // close: sell futures, buy back spot
	} else {
		a.trade(ctx, out, client.Buy, client.Sell) // close: buy back futures, sell spot
	}
	a.inPosition = false
	fmt.Fprintf(out, "arb: closed position (deviation %d)\n", deviation)
}

// Reset implements bots.Resetter: a restarted exchange holds no positions, so any tracked leg is stale.
func (a *Arbitrage) Reset(ctx context.Context, out io.Writer) {
	a.inPosition = false
	fmt.Fprintln(out, "detected exchange restart, resetting arbitrage state")
}

// ActFailed implements bots.FailureReporter.
func (a *Arbitrage) ActFailed() bool { return a.lastFailed }

// trade submits the futures leg at futuresSide and the spot leg at spotSide, same quantity each.
func (a *Arbitrage) trade(ctx context.Context, out io.Writer, futuresSide, spotSide client.Side) {
	if _, _, err := a.c.SubmitOrder(ctx, a.cfg.FuturesSymbol, futuresSide, client.Market, 0, a.cfg.Quantity); err != nil {
		fmt.Fprintln(out, "error submitting futures leg:", client.FriendlyError(err))
		a.lastFailed = true
	}
	if _, _, err := a.c.SubmitOrder(ctx, a.cfg.SpotSymbol, spotSide, client.Market, 0, a.cfg.Quantity); err != nil {
		fmt.Fprintln(out, "error submitting spot leg:", client.FriendlyError(err))
		a.lastFailed = true
	}
}

// mid returns symbol's top-of-book mid-price, and false if the RPC failed or either side is currently empty.
func (a *Arbitrage) mid(ctx context.Context, symbol string) (int64, bool) {
	book, err := a.c.GetOrderBook(ctx, symbol, 1)
	if err != nil {
		a.lastFailed = true
		return 0, false
	}
	if len(book.Bids) == 0 || len(book.Asks) == 0 {
		return 0, false
	}
	return (book.Bids[0].Price + book.Asks[0].Price) / 2, true
}
