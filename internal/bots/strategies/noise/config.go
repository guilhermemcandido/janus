package noise

import (
	"errors"
	"time"
)

// Config holds the tunable parameters for the noise-trader strategy.
type Config struct {
	Symbols  []string
	Quantity uint64
	Interval time.Duration
}

// Validate checks that cfg has usable values.
func (cfg Config) Validate() error {
	if len(cfg.Symbols) == 0 {
		return errors.New("at least one symbol is required")
	}
	if cfg.Quantity == 0 {
		return errors.New("quantity must be positive")
	}
	if cfg.Interval <= 0 {
		return errors.New("interval must be positive")
	}
	return nil
}
