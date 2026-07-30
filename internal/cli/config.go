package cli

import "errors"

// RequireSymbol validates that a symbol was provided.
func RequireSymbol(symbol string) error {
	if symbol == "" {
		return errors.New("a symbol is required, e.g. `janus AAPL ...` (which instrument would you like to trade?)")
	}
	return nil
}
