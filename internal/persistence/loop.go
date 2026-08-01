package persistence

import (
	"context"
	"log"
	"time"

	"github.com/guilhermemcandido/janus/internal/engine"
)

// SafeSave saves ex to path, recovering from any panic and logging success or failure instead of propagating.
func SafeSave(ex *engine.Exchange, path string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("snapshot panicked: %v", r)
		}
	}()
	if err := Save(ex, path); err != nil {
		log.Printf("snapshot failed: %v", err)
		return
	}
	log.Printf("snapshot saved to %s", path)
}

// RunLoop calls SafeSave every interval until ctx is cancelled.
func RunLoop(ctx context.Context, ex *engine.Exchange, path string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			SafeSave(ex, path)
		}
	}
}
