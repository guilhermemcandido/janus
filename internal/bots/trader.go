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

// Closer is an optional Strategy extension for cleanup before Trader.Run returns, checked for via type assertion, like io.Closer.
type Closer interface {
	Close(ctx context.Context, out io.Writer)
}

// Resetter is an optional Strategy extension for reacting to a detected exchange restart.
type Resetter interface {
	Reset(ctx context.Context, out io.Writer)
}

// FailureReporter is an optional Strategy extension reporting whether its most recent Act cycle hit an error.
type FailureReporter interface {
	ActFailed() bool
}

// Pinger reports the exchange's current epoch, which changes across a restart.
type Pinger interface {
	Ping(ctx context.Context) (epoch uint64, err error)
}

// shutdownTimeout bounds a Closer's cleanup so a stalled connection can't hang shutdown forever.
const shutdownTimeout = 5 * time.Second

// Trader runs a Strategy on a fixed interval until ctx is cancelled.
type Trader struct {
	strategy Strategy
	interval time.Duration
	pinger   Pinger

	epoch      uint64
	epochKnown bool
}

// NewTrader creates a Trader that runs strategy every interval, using pinger to detect exchange
// restarts between cycles. pinger may be nil, in which case restart detection is skipped.
func NewTrader(strategy Strategy, interval time.Duration, pinger Pinger) *Trader {
	return &Trader{strategy: strategy, interval: interval, pinger: pinger}
}

// Run calls strategy.Act every interval until ctx is cancelled, then calls Close if strategy implements Closer.
func (t *Trader) Run(ctx context.Context, out io.Writer) error {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	// Seed a baseline epoch up front - otherwise the first failure-gated ping below has nothing to compare against.
	t.checkRestart(ctx, out)

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
			t.tick(ctx, out)
		}
	}
}

// tick runs one Act cycle, pinging first only if the previous cycle failed, since that's the only time a restart is a plausible cause.
func (t *Trader) tick(ctx context.Context, out io.Writer) {
	if reporter, ok := t.strategy.(FailureReporter); ok && reporter.ActFailed() {
		t.checkRestart(ctx, out)
	}
	if err := t.strategy.Act(ctx, out); err != nil {
		fmt.Fprintln(out, "error:", err)
	}
}

// checkRestart calls strategy.Reset if the exchange's epoch changed since the last successful ping.
func (t *Trader) checkRestart(ctx context.Context, out io.Writer) {
	if t.pinger == nil {
		return
	}
	epoch, err := t.pinger.Ping(ctx)
	if err != nil {
		return
	}
	if t.epochKnown && epoch != t.epoch {
		if resetter, ok := t.strategy.(Resetter); ok {
			resetter.Reset(ctx, out)
		}
	}
	t.epoch, t.epochKnown = epoch, true
}
