package bots

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Strategy is one decision-and-trade cycle for a taker-style bot.
type Strategy interface {
	Act(ctx context.Context, out io.Writer) error
}

// Trader runs a Strategy on a fixed interval until ctx is cancelled.
type Trader struct {
	strategy Strategy
	interval time.Duration
}

// NewTrader creates a Trader that runs strategy every interval.
func NewTrader(strategy Strategy, interval time.Duration) *Trader {
	return &Trader{strategy: strategy, interval: interval}
}

// Run calls strategy.Act every interval until ctx is cancelled.
func (t *Trader) Run(ctx context.Context, out io.Writer) error {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := t.strategy.Act(ctx, out); err != nil {
				fmt.Fprintln(out, "error:", err)
			}
		}
	}
}
