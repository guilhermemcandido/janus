package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/guilhermemcandido/janus/internal/engine"
	"github.com/guilhermemcandido/janus/internal/types"
)

// saveMu serializes Save calls: two concurrent writers to the same path's temp file would
// otherwise race on write/rename, potentially losing or corrupting one of them.
var saveMu sync.Mutex

// Snapshot is the on-disk representation of every symbol's resting orders.
type Snapshot struct {
	Books []BookState
}

// BookState is one symbol's resting orders and sequence counter.
type BookState struct {
	Symbol string
	Bids   []types.Order
	Asks   []types.Order
	Seq    uint64
}

// Save captures every symbol currently known to ex and atomically writes it to path.
func Save(ex *engine.Exchange, path string) error {
	saveMu.Lock()
	defer saveMu.Unlock()

	var snap Snapshot
	for _, symbol := range ex.Symbols() {
		bids, asks, seq := ex.GetOrCreateEngine(symbol).RestingOrders()
		snap.Books = append(snap.Books, BookState{Symbol: symbol, Bids: bids, Asks: asks, Seq: seq})
	}

	data, err := json.MarshalIndent(&snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open temp snapshot: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write temp snapshot: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync temp snapshot: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp snapshot: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename snapshot: %w", err)
	}
	return nil
}

// Load reads and parses the snapshot at path. A missing file returns a nil Snapshot and no error.
func Load(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return &snap, nil
}

// Restore seeds ex with every book in snap. A nil snap is a no-op.
func Restore(ex *engine.Exchange, snap *Snapshot) {
	if snap == nil {
		return
	}
	for _, book := range snap.Books {
		ex.GetOrCreateEngine(book.Symbol).Restore(book.Bids, book.Asks, book.Seq)
	}
}
