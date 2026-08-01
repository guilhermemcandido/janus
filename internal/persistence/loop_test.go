package persistence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/guilhermemcandido/janus/internal/engine"
)

func TestRunLoop_SavesPeriodicallyUntilCancelled(t *testing.T) {
	ex := engine.NewExchange()
	defer ex.Close()
	ex.GetOrCreateEngine("AAPL")

	path := filepath.Join(t.TempDir(), "exchange.snapshot.json")
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		RunLoop(ctx, ex, path, 20*time.Millisecond)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("snapshot file never appeared at %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("RunLoop did not return after ctx was cancelled")
	}
}

func TestSafeSave_DoesNotPanicOnError(t *testing.T) {
	ex := engine.NewExchange()
	defer ex.Close()

	// A path whose parent is a regular file, not a directory, makes MkdirAll fail inside Save.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	SafeSave(ex, filepath.Join(blocker, "exchange.snapshot.json"))
}
