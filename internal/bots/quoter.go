package bots

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/guilhermemcandido/janus/pkg/client"
)

// PriceSource supplies the next reference price for a quoting cycle.
type PriceSource interface {
	Price(ctx context.Context) (int64, error)
}

// Quoter continuously quotes both sides of a book around a reference price. It implements
// Strategy (each Act is one requote cycle) and Closer (cancels resting quotes on shutdown).
type Quoter struct {
	c      *client.Client
	cfg    Config
	source PriceSource

	bidID uint64
	askID uint64

	lastFailed bool
}

// NewQuoter creates a Quoter that quotes cfg.Symbol using source for its reference price.
func NewQuoter(c *client.Client, cfg Config, source PriceSource) *Quoter {
	return &Quoter{c: c, cfg: cfg, source: source}
}

// Act implements Strategy: cancels old quotes before submitting new ones, since the reverse
// order risks a self-trade if the reference price moved by more than the spread. A side is only
// resubmitted if its old quote was actually cancelled (or already gone) - otherwise its state is
// unknown and resubmitting would risk stacking a duplicate quote on top of it.
func (q *Quoter) Act(ctx context.Context, out io.Writer) error {
	q.lastFailed = false

	ref, err := q.source.Price(ctx)
	if err != nil {
		fmt.Fprintln(out, "error getting reference price:", err)
		q.lastFailed = true
		return nil
	}

	bidPrice := ref - q.cfg.HalfSpread
	askPrice := ref + q.cfg.HalfSpread

	var bidTrades, askTrades []client.Trade
	submittedBid, submittedAsk := false, false

	if q.cancelIfResting(ctx, &q.bidID, out) {
		bid, trades, err := q.c.SubmitOrder(ctx, q.cfg.Symbol, client.Buy, client.Limit, bidPrice, q.cfg.Quantity)
		if err != nil {
			fmt.Fprintln(out, "error submitting bid:", err)
			q.lastFailed = true
		} else {
			q.bidID, bidTrades, submittedBid = bid.ID, trades, true
		}
	} else {
		q.lastFailed = true
	}

	if q.cancelIfResting(ctx, &q.askID, out) {
		ask, trades, err := q.c.SubmitOrder(ctx, q.cfg.Symbol, client.Sell, client.Limit, askPrice, q.cfg.Quantity)
		if err != nil {
			fmt.Fprintln(out, "error submitting ask:", err)
			q.lastFailed = true
		} else {
			q.askID, askTrades, submittedAsk = ask.ID, trades, true
		}
	} else {
		q.lastFailed = true
	}

	if submittedBid || submittedAsk {
		fmt.Fprintf(out, "quoting %s: bid %d x %d / ask %d x %d\n", q.cfg.Symbol, q.cfg.Quantity, bidPrice, q.cfg.Quantity, askPrice)
	}
	for _, tr := range bidTrades {
		fmt.Fprintf(out, "  bid matched %d @ %d (maker %d)\n", tr.Quantity, tr.Price, tr.MakerOrderID)
	}
	for _, tr := range askTrades {
		fmt.Fprintf(out, "  ask matched %d @ %d (maker %d)\n", tr.Quantity, tr.Price, tr.MakerOrderID)
	}
	return nil
}

// Close implements Closer, cancelling any resting quotes before Trader.Run returns.
func (q *Quoter) Close(ctx context.Context, out io.Writer) {
	q.cancelIfResting(ctx, &q.bidID, out)
	q.cancelIfResting(ctx, &q.askID, out)
}

// Reset implements bots.Resetter: a restarted exchange no longer has these orders, so cancelling them is pointless.
func (q *Quoter) Reset(ctx context.Context, out io.Writer) {
	q.bidID, q.askID = 0, 0
	fmt.Fprintln(out, "detected exchange restart, resetting quoter state")
}

// ActFailed implements bots.FailureReporter.
func (q *Quoter) ActFailed() bool { return q.lastFailed }

// cancelIfResting cancels the resting order at *id, if any, tolerating ones already filled.
// Returns whether it's now safe to submit a replacement: false only when a real cancel error
// left the old order's resting state unknown.
func (q *Quoter) cancelIfResting(ctx context.Context, id *uint64, out io.Writer) bool {
	if *id == 0 {
		return true
	}
	if _, err := q.c.CancelOrder(ctx, q.cfg.Symbol, *id); err != nil && status.Code(err) != codes.NotFound {
		fmt.Fprintln(out, "error cancelling order", *id, ":", err)
		return false
	}
	*id = 0
	return true
}
