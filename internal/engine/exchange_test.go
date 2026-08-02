package engine

import (
	"sync"
	"testing"
)

func TestExchange_EngineIsStablePerSymbol(t *testing.T) {
	ex := NewExchange()
	defer ex.Close()

	a := ex.Register("AAPL", "Apple Inc.")
	b := ex.Register("AAPL", "Apple Inc.")
	if a != b {
		t.Fatalf("Register(\"AAPL\") returned different instances on second call")
	}
}

func TestExchange_RegisterIsIdempotentOnDescription(t *testing.T) {
	ex := NewExchange()
	defer ex.Close()

	ex.Register("AAPL", "Apple Inc.")
	ex.Register("AAPL", "something else")

	if got := ex.Description("AAPL"); got != "Apple Inc." {
		t.Fatalf("Description = %q, want the first registration's description to win", got)
	}
}

func TestExchange_LookupFindsRegisteredSymbol(t *testing.T) {
	ex := NewExchange()
	defer ex.Close()

	registered := ex.Register("AAPL", "Apple Inc.")

	found, ok := ex.Lookup("AAPL")
	if !ok || found != registered {
		t.Fatalf("Lookup(\"AAPL\") = (%v, %v), want the registered engine and true", found, ok)
	}
}

func TestExchange_LookupMissesUnregisteredSymbol(t *testing.T) {
	ex := NewExchange()
	defer ex.Close()

	if _, ok := ex.Lookup("AAPL"); ok {
		t.Fatalf("Lookup(\"AAPL\") = ok, want false for a never-registered symbol")
	}
}

func TestExchange_DifferentSymbolsGetDifferentEngines(t *testing.T) {
	ex := NewExchange()
	defer ex.Close()

	aapl := ex.Register("AAPL", "Apple Inc.")
	tsla := ex.Register("TSLA", "Tesla Inc.")
	if aapl == tsla {
		t.Fatalf("expected distinct engines for distinct symbols")
	}
	if aapl.Symbol() != "AAPL" || tsla.Symbol() != "TSLA" {
		t.Fatalf("engine Symbol not set correctly: got %q and %q", aapl.Symbol(), tsla.Symbol())
	}
}

func TestExchange_RegisterIsRaceFree(t *testing.T) {
	ex := NewExchange()
	defer ex.Close()

	const goroutines = 50
	engines := make([]*Engine, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			engines[i] = ex.Register("AAPL", "Apple Inc.")
		}(i)
	}
	wg.Wait()

	for i := 1; i < goroutines; i++ {
		if engines[i] != engines[0] {
			t.Fatalf("goroutine %d got a different Engine instance than goroutine 0 for the same symbol", i)
		}
	}
}
