package bots

import (
	"errors"
	"time"
)

// Config holds the tunable parameters shared by any quoting bot, regardless of price source.
type Config struct {
	Symbol     string
	Quantity   uint64
	HalfSpread int64
	Interval   time.Duration
}

// Validate checks that cfg has usable values.
func (cfg Config) Validate() error {
	if cfg.Symbol == "" {
		return errors.New("a symbol is required")
	}
	if cfg.Quantity == 0 {
		return errors.New("quantity must be positive")
	}
	if cfg.HalfSpread <= 0 {
		return errors.New("half-spread must be positive")
	}
	if cfg.Interval <= 0 {
		return errors.New("interval must be positive")
	}
	return nil
}
