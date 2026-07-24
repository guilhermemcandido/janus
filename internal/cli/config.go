package cli

import "errors"

// RequireSymbol validates that a symbol was provided.
func RequireSymbol(symbol string) error {
	if symbol == "" {
		return errors.New("-symbol is required (which instrument would you like to trade?)")
	}
	return nil
}
