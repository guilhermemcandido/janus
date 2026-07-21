package engine

import "testing"

func TestExchange_BookIsStablePerSymbol(t *testing.T) {
	ex := NewExchange()
	a := ex.Book("AAPL")
	b := ex.Book("AAPL")
	if a != b {
		t.Fatalf("Book(\"AAPL\") returned different instances on second call")
	}
}

func TestExchange_DifferentSymbolsGetDifferentBooks(t *testing.T) {
	ex := NewExchange()
	aapl := ex.Book("AAPL")
	tsla := ex.Book("TSLA")
	if aapl == tsla {
		t.Fatalf("expected distinct books for distinct symbols")
	}
	if aapl.Symbol != "AAPL" || tsla.Symbol != "TSLA" {
		t.Fatalf("book Symbol not set correctly: got %q and %q", aapl.Symbol, tsla.Symbol)
	}
}
