package engine

import (
	"sync"
	"testing"
)

func TestExchange_EngineIsStablePerSymbol(t *testing.T) {
	ex := NewExchange()
	defer ex.Close()

	a := ex.GetOrCreateEngine("AAPL")
	b := ex.GetOrCreateEngine("AAPL")
	if a != b {
		t.Fatalf("GetOrCreateEngine(\"AAPL\") returned different instances on second call")
	}
}

func TestExchange_DifferentSymbolsGetDifferentEngines(t *testing.T) {
	ex := NewExchange()
	defer ex.Close()

	aapl := ex.GetOrCreateEngine("AAPL")
	tsla := ex.GetOrCreateEngine("TSLA")
	if aapl == tsla {
		t.Fatalf("expected distinct engines for distinct symbols")
	}
	if aapl.Symbol() != "AAPL" || tsla.Symbol() != "TSLA" {
		t.Fatalf("engine Symbol not set correctly: got %q and %q", aapl.Symbol(), tsla.Symbol())
	}
}

func TestExchange_GetOrCreateEngineIsRaceFree(t *testing.T) {
	ex := NewExchange()
	defer ex.Close()

	const goroutines = 50
	engines := make([]*Engine, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			engines[i] = ex.GetOrCreateEngine("AAPL")
		}(i)
	}
	wg.Wait()

	for i := 1; i < goroutines; i++ {
		if engines[i] != engines[0] {
			t.Fatalf("goroutine %d got a different Engine instance than goroutine 0 for the same symbol", i)
		}
	}
}
