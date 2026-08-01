package bots

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

type fakePinger struct {
	epochs []uint64
	call   int
	err    error
}

func (f *fakePinger) Ping(ctx context.Context) (uint64, error) {
	if f.err != nil {
		return 0, f.err
	}
	e := f.epochs[f.call]
	if f.call < len(f.epochs)-1 {
		f.call++
	}
	return e, nil
}

type fakeStrategy struct {
	resetCalls int
}

func (f *fakeStrategy) Act(ctx context.Context, out io.Writer) error { return nil }
func (f *fakeStrategy) Reset(ctx context.Context, out io.Writer)     { f.resetCalls++ }

// fakeStrategyNoReset implements Strategy but not Resetter.
type fakeStrategyNoReset struct{}

func (fakeStrategyNoReset) Act(ctx context.Context, out io.Writer) error { return nil }

// flakyStrategy implements FailureReporter; tests set failed directly to simulate what a real
// bot would track internally after an Act cycle.
type flakyStrategy struct {
	failed bool
}

func (f *flakyStrategy) Act(ctx context.Context, out io.Writer) error { return nil }
func (f *flakyStrategy) ActFailed() bool                              { return f.failed }

type countingPinger struct {
	epoch uint64
	calls int
}

func (p *countingPinger) Ping(ctx context.Context) (uint64, error) {
	p.calls++
	return p.epoch, nil
}

func TestTrader_CheckRestartCallsResetOnEpochChange(t *testing.T) {
	strategy := &fakeStrategy{}
	pinger := &fakePinger{epochs: []uint64{1, 1, 2, 2}}
	tr := NewTrader(strategy, time.Second, pinger)

	for i := 0; i < 4; i++ {
		tr.checkRestart(context.Background(), &bytes.Buffer{})
	}

	if strategy.resetCalls != 1 {
		t.Fatalf("resetCalls = %d, want 1 (only when the epoch actually changed)", strategy.resetCalls)
	}
}

func TestTrader_CheckRestartSkipsStrategyWithoutResetter(t *testing.T) {
	pinger := &fakePinger{epochs: []uint64{1, 2}}
	tr := NewTrader(fakeStrategyNoReset{}, time.Second, pinger)

	tr.checkRestart(context.Background(), &bytes.Buffer{})
	tr.checkRestart(context.Background(), &bytes.Buffer{}) // must not panic despite the epoch changing
}

func TestTrader_CheckRestartIgnoresPingError(t *testing.T) {
	strategy := &fakeStrategy{}
	pinger := &fakePinger{err: errors.New("unavailable")}
	tr := NewTrader(strategy, time.Second, pinger)

	tr.checkRestart(context.Background(), &bytes.Buffer{})

	if tr.epochKnown {
		t.Fatalf("epochKnown = true after a failed ping, want false")
	}
	if strategy.resetCalls != 0 {
		t.Fatalf("resetCalls = %d after a failed ping, want 0", strategy.resetCalls)
	}
}

func TestTrader_TickPingsOnlyAfterActFailure(t *testing.T) {
	pinger := &countingPinger{epoch: 1}
	strategy := &flakyStrategy{}
	tr := NewTrader(strategy, time.Second, pinger)

	tr.tick(context.Background(), &bytes.Buffer{}) // last cycle didn't fail
	if pinger.calls != 0 {
		t.Fatalf("calls = %d, want 0 (no ping when the last cycle didn't fail)", pinger.calls)
	}

	strategy.failed = true // simulate the last Act cycle failing
	tr.tick(context.Background(), &bytes.Buffer{})
	if pinger.calls != 1 {
		t.Fatalf("calls = %d, want 1 (ping expected after a failed cycle)", pinger.calls)
	}

	strategy.failed = false // simulate this cycle succeeding
	tr.tick(context.Background(), &bytes.Buffer{})
	if pinger.calls != 1 {
		t.Fatalf("calls = %d, want 1 (no ping after a successful cycle)", pinger.calls)
	}
}

func TestTrader_TickSkipsPingForStrategyWithoutFailureReporter(t *testing.T) {
	pinger := &countingPinger{epoch: 1}
	tr := NewTrader(fakeStrategyNoReset{}, time.Second, pinger)

	tr.tick(context.Background(), &bytes.Buffer{})

	if pinger.calls != 0 {
		t.Fatalf("calls = %d, want 0 (strategy doesn't implement FailureReporter)", pinger.calls)
	}
}

func TestTrader_RunSeedsEpochBaselineBeforeFirstTick(t *testing.T) {
	pinger := &countingPinger{epoch: 5}
	tr := NewTrader(&fakeStrategy{}, time.Hour, pinger) // long interval: no tick fires during the test

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := tr.Run(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if pinger.calls != 1 {
		t.Fatalf("calls = %d, want 1 (Run should ping once at startup to seed a baseline epoch)", pinger.calls)
	}
	if !tr.epochKnown || tr.epoch != 5 {
		t.Fatalf("epoch state = (%d, known=%v), want (5, true)", tr.epoch, tr.epochKnown)
	}
}

func TestTrader_CheckRestartNoopWithNilPinger(t *testing.T) {
	strategy := &fakeStrategy{}
	tr := NewTrader(strategy, time.Second, nil)

	tr.checkRestart(context.Background(), &bytes.Buffer{})

	if strategy.resetCalls != 0 {
		t.Fatalf("resetCalls = %d with a nil pinger, want 0", strategy.resetCalls)
	}
}
