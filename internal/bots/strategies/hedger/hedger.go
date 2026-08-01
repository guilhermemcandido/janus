package hedger

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"

	"github.com/guilhermemcandido/janus/pkg/client"
)

// Hedger takes random flow positions in futures and hedges via spot, tracking one combined
// exposure number rather than separate books, since the two instruments move together.
type Hedger struct {
	c        *client.Client
	cfg      Config
	position int64

	lastFailed bool
}

// New creates a Hedger for cfg, trading through c.
func New(c *client.Client, cfg Config) *Hedger {
	return &Hedger{c: c, cfg: cfg}
}

// Act implements bots.Strategy.
func (h *Hedger) Act(ctx context.Context, out io.Writer) error {
	h.lastFailed = false
	h.takeFlow(ctx, out)
	h.hedgeIfNeeded(ctx, out)
	return nil
}

func (h *Hedger) takeFlow(ctx context.Context, out io.Writer) {
	side := client.Buy
	if rand.IntN(2) == 1 {
		side = client.Sell
	}

	order, _, err := h.c.SubmitOrder(ctx, h.cfg.FuturesSymbol, side, client.Market, 0, h.cfg.FlowQuantity)
	if err != nil {
		fmt.Fprintln(out, "error taking flow position:", err)
		h.lastFailed = true
		return
	}
	filled := h.cfg.FlowQuantity - order.Remaining
	if filled == 0 {
		return
	}
	if side == client.Buy {
		h.position += int64(filled)
	} else {
		h.position -= int64(filled)
	}
	fmt.Fprintf(out, "flow: %s %d %s (net position %d)\n", sideName(side), filled, h.cfg.FuturesSymbol, h.position)
}

func (h *Hedger) hedgeIfNeeded(ctx context.Context, out io.Writer) {
	threshold := h.cfg.HedgeThreshold
	if h.position > threshold {
		h.hedge(ctx, out, client.Sell, uint64(h.position))
	} else if h.position < -threshold {
		h.hedge(ctx, out, client.Buy, uint64(-h.position))
	}
}

func (h *Hedger) hedge(ctx context.Context, out io.Writer, side client.Side, qty uint64) {
	order, _, err := h.c.SubmitOrder(ctx, h.cfg.SpotSymbol, side, client.Market, 0, qty)
	if err != nil {
		fmt.Fprintln(out, "error hedging:", err)
		h.lastFailed = true
		return
	}
	filled := qty - order.Remaining
	if filled == 0 {
		return
	}
	if side == client.Buy {
		h.position += int64(filled)
	} else {
		h.position -= int64(filled)
	}
	fmt.Fprintf(out, "hedge: %s %d %s (net position %d)\n", sideName(side), filled, h.cfg.SpotSymbol, h.position)
}

// Reset implements bots.Resetter: a restarted exchange holds no fills, so the tracked position is stale.
func (h *Hedger) Reset(ctx context.Context, out io.Writer) {
	h.position = 0
	fmt.Fprintln(out, "detected exchange restart, resetting hedger position")
}

// ActFailed implements bots.FailureReporter.
func (h *Hedger) ActFailed() bool { return h.lastFailed }

func sideName(s client.Side) string {
	if s == client.Sell {
		return "sell"
	}
	return "buy"
}
