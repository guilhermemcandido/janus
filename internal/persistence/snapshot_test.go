package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/guilhermemcandido/janus/internal/engine"
	"github.com/guilhermemcandido/janus/internal/types"
)

func TestSave_LoadRoundTripPreservesRestingOrders(t *testing.T) {
	ex := engine.NewExchange()
	defer ex.Close()

	eng := ex.GetOrCreateEngine("AAPL")
	if _, err := eng.Submit(types.NewOrder("AAPL", types.Buy, types.Limit, 100, 10)); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	if _, err := eng.Submit(types.NewOrder("AAPL", types.Sell, types.Limit, 105, 5)); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	path := filepath.Join(t.TempDir(), "exchange.snapshot.json")
	if err := Save(ex, path); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	snap, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(snap.Books) != 1 || snap.Books[0].Symbol != "AAPL" {
		t.Fatalf("Books = %+v, want one book for AAPL", snap.Books)
	}
	book := snap.Books[0]
	if len(book.Bids) != 1 || book.Bids[0].Price != 100 || book.Bids[0].Remaining != 10 {
		t.Fatalf("Bids = %+v, want [{Price:100 Remaining:10}]", book.Bids)
	}
	if len(book.Asks) != 1 || book.Asks[0].Price != 105 || book.Asks[0].Remaining != 5 {
		t.Fatalf("Asks = %+v, want [{Price:105 Remaining:5}]", book.Asks)
	}
}

func TestSave_LeavesNoTempFileBehind(t *testing.T) {
	ex := engine.NewExchange()
	defer ex.Close()
	ex.GetOrCreateEngine("AAPL")

	dir := t.TempDir()
	path := filepath.Join(dir, "exchange.snapshot.json")
	if err := Save(ex, path); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "exchange.snapshot.json" {
		t.Fatalf("dir entries = %v, want exactly the final snapshot file", entries)
	}
}

func TestLoad_MissingFileReturnsNilWithoutError(t *testing.T) {
	snap, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if snap != nil {
		t.Fatalf("snap = %+v, want nil", snap)
	}
}

func TestRestore_SeedsExchangeFromSnapshot(t *testing.T) {
	source := engine.NewExchange()
	defer source.Close()
	if _, err := source.GetOrCreateEngine("AAPL").Submit(types.NewOrder("AAPL", types.Buy, types.Limit, 100, 10)); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	path := filepath.Join(t.TempDir(), "exchange.snapshot.json")
	if err := Save(source, path); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	snap, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	restored := engine.NewExchange()
	defer restored.Close()
	Restore(restored, snap)

	book := restored.GetOrCreateEngine("AAPL").Depth(10)
	if len(book.Bids) != 1 || book.Bids[0].Price != 100 || book.Bids[0].Quantity != 10 {
		t.Fatalf("Bids = %+v, want [{Price:100 Quantity:10}]", book.Bids)
	}
}

func TestRestore_NilSnapshotIsNoop(t *testing.T) {
	ex := engine.NewExchange()
	defer ex.Close()

	Restore(ex, nil)

	if len(ex.Symbols()) != 0 {
		t.Fatalf("Symbols() = %v, want none", ex.Symbols())
	}
}
