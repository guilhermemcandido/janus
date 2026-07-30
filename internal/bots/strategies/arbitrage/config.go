package arbitrage

import (
	"errors"
	"time"
)

// Config holds the tunable parameters for the spot/futures arbitrage strategy.
type Config struct {
	SpotSymbol     string
	FuturesSymbol  string
	FairBasis      int64
	EntryThreshold int64
	Quantity       uint64
	Interval       time.Duration
}

// Validate checks that cfg has usable values.
func (cfg Config) Validate() error {
	if cfg.SpotSymbol == "" {
		return errors.New("a spot symbol is required")
	}
	if cfg.FuturesSymbol == "" {
		return errors.New("a futures symbol is required")
	}
	if cfg.FuturesSymbol == cfg.SpotSymbol {
		return errors.New("futures and spot symbols must be different")
	}
	if cfg.EntryThreshold <= 0 {
		return errors.New("entry threshold must be positive")
	}
	if cfg.Quantity == 0 {
		return errors.New("quantity must be positive")
	}
	if cfg.Interval <= 0 {
		return errors.New("interval must be positive")
	}
	return nil
}
