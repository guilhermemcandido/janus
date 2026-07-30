package bots

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Strategy is one decision-and-trade cycle, run repeatedly by a Trader.
type Strategy interface {
	Act(ctx context.Context, out io.Writer) error
}

// Closer is an optional Strategy extension for cleanup before Trader.Run returns,
// e.g. cancelling resting orders. Checked for via type assertion, like io.Closer.
type Closer interface {
	Close(ctx context.Context, out io.Writer)
}

// shutdownTimeout bounds a Closer's cleanup so a stalled connection can't hang shutdown forever.
const shutdownTimeout = 5 * time.Second

// Trader runs a Strategy on a fixed interval until ctx is cancelled.
type Trader struct {
	strategy Strategy
	interval time.Duration
}

// NewTrader creates a Trader that runs strategy every interval.
func NewTrader(strategy Strategy, interval time.Duration) *Trader {
	return &Trader{strategy: strategy, interval: interval}
}

// Run calls strategy.Act every interval until ctx is cancelled, then calls Close if strategy implements Closer.
func (t *Trader) Run(ctx context.Context, out io.Writer) error {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if closer, ok := t.strategy.(Closer); ok {
				closeCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
				closer.Close(closeCtx, out)
				cancel()
			}
			return nil
		case <-ticker.C:
			if err := t.strategy.Act(ctx, out); err != nil {
				fmt.Fprintln(out, "error:", err)
			}
		}
	}
}
