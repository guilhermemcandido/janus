package bots

import (
	"context"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/guilhermemcandido/janus/pkg/client"
)

// PriceSource supplies the next reference price for a quoting cycle.
type PriceSource interface {
	Price(ctx context.Context) (int64, error)
}

// Bot continuously quotes both sides of a book around a reference price.
type Bot struct {
	c      *client.Client
	cfg    Config
	source PriceSource

	bidID uint64
	askID uint64
}

// New creates a Bot that quotes cfg.Symbol using source for its reference price.
func New(c *client.Client, cfg Config, source PriceSource) *Bot {
	return &Bot{c: c, cfg: cfg, source: source}
}

// Run quotes on cfg.Interval until ctx is cancelled, then cancels any resting quotes before returning.
func (b *Bot) Run(ctx context.Context, out io.Writer) error {
	ticker := time.NewTicker(b.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.cancelQuotes(context.Background(), out)
			return nil
		case <-ticker.C:
			b.requote(ctx, out)
		}
	}
}

// requote replaces the bot's resting quotes with fresh ones around the next reference price.
// Cancels old quotes before submitting new ones; the reverse order risks a self-trade.
func (b *Bot) requote(ctx context.Context, out io.Writer) {
	ref, err := b.source.Price(ctx)
	if err != nil {
		fmt.Fprintln(out, "error getting reference price:", err)
		return
	}

	b.cancelQuotes(ctx, out)

	bidPrice := ref - b.cfg.HalfSpread
	askPrice := ref + b.cfg.HalfSpread

	bid, bidTrades, err := b.c.SubmitOrder(ctx, b.cfg.Symbol, client.Buy, client.Limit, bidPrice, b.cfg.Quantity)
	if err != nil {
		fmt.Fprintln(out, "error submitting bid:", err)
	} else {
		b.bidID = bid.ID
	}

	ask, askTrades, err := b.c.SubmitOrder(ctx, b.cfg.Symbol, client.Sell, client.Limit, askPrice, b.cfg.Quantity)
	if err != nil {
		fmt.Fprintln(out, "error submitting ask:", err)
	} else {
		b.askID = ask.ID
	}

	fmt.Fprintf(out, "quoting %s: bid %d x %d / ask %d x %d\n", b.cfg.Symbol, b.cfg.Quantity, bidPrice, b.cfg.Quantity, askPrice)
	for _, tr := range bidTrades {
		fmt.Fprintf(out, "  bid matched %d @ %d (maker %d)\n", tr.Quantity, tr.Price, tr.MakerOrderID)
	}
	for _, tr := range askTrades {
		fmt.Fprintf(out, "  ask matched %d @ %d (maker %d)\n", tr.Quantity, tr.Price, tr.MakerOrderID)
	}
}

// cancelQuotes cancels both resting quotes, if any, tolerating ones already filled.
func (b *Bot) cancelQuotes(ctx context.Context, out io.Writer) {
	b.cancelIfResting(ctx, &b.bidID, out)
	b.cancelIfResting(ctx, &b.askID, out)
}

func (b *Bot) cancelIfResting(ctx context.Context, id *uint64, out io.Writer) {
	if *id == 0 {
		return
	}
	if _, err := b.c.CancelOrder(ctx, b.cfg.Symbol, *id); err != nil && status.Code(err) != codes.NotFound {
		fmt.Fprintln(out, "error cancelling order", *id, ":", err)
	}
	*id = 0
}
