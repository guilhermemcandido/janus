package cli

import "testing"

func TestRequireSymbol(t *testing.T) {
	if err := RequireSymbol("AAPL"); err != nil {
		t.Fatalf("RequireSymbol(\"AAPL\") = %v, want nil", err)
	}
	if err := RequireSymbol(""); err == nil {
		t.Fatalf("RequireSymbol(\"\") = nil, want an error")
	}
}
