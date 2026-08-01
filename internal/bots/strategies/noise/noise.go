package noise

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"

	"github.com/guilhermemcandido/janus/pkg/client"
)

// Noise submits random-direction market orders on random symbols, with no view and no state,
// standing in for uninformed retail-style flow that adds volume without any signal.
type Noise struct {
	c   *client.Client
	cfg Config
}

// New creates a Noise trader for cfg, trading through c.
func New(c *client.Client, cfg Config) *Noise {
	return &Noise{c: c, cfg: cfg}
}

// Act implements bots.Strategy.
func (n *Noise) Act(ctx context.Context, out io.Writer) error {
	symbol := n.cfg.Symbols[rand.IntN(len(n.cfg.Symbols))]
	side := client.Buy
	if rand.IntN(2) == 1 {
		side = client.Sell
	}

	order, trades, err := n.c.SubmitOrder(ctx, symbol, side, client.Market, 0, n.cfg.Quantity)
	if err != nil {
		fmt.Fprintln(out, "error submitting noise trade:", client.FriendlyError(err))
		return nil
	}
	filled := n.cfg.Quantity - order.Remaining
	if filled == 0 {
		return nil
	}
	fmt.Fprintf(out, "noise: %s %d %s\n", sideName(side), filled, symbol)
	for _, tr := range trades {
		fmt.Fprintf(out, "  matched %d @ %d (maker %d)\n", tr.Quantity, tr.Price, tr.MakerOrderID)
	}
	return nil
}

func sideName(s client.Side) string {
	if s == client.Sell {
		return "sell"
	}
	return "buy"
}
