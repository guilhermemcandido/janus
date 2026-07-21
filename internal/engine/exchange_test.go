package engine

import "testing"

func TestExchange_BookIsStablePerSymbol(t *testing.T) {
	ex := NewExchange()
	a := ex.GetOrCreateBook("AAPL")
	b := ex.GetOrCreateBook("AAPL")
	if a != b {
		t.Fatalf("GetOrCreateBook(\"AAPL\") returned different instances on second call")
	}
}

func TestExchange_DifferentSymbolsGetDifferentBooks(t *testing.T) {
	ex := NewExchange()
	aapl := ex.GetOrCreateBook("AAPL")
	tsla := ex.GetOrCreateBook("TSLA")
	if aapl == tsla {
		t.Fatalf("expected distinct books for distinct symbols")
	}
	if aapl.Symbol != "AAPL" || tsla.Symbol != "TSLA" {
		t.Fatalf("book Symbol not set correctly: got %q and %q", aapl.Symbol, tsla.Symbol)
	}
}
