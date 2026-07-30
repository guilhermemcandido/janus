package hedger

import (
	"errors"
	"time"
)

// Config holds the tunable parameters for the flow+hedge strategy.
type Config struct {
	FuturesSymbol  string
	SpotSymbol     string
	FlowQuantity   uint64
	Interval       time.Duration
	HedgeThreshold int64
}

// Validate checks that cfg has usable values.
func (cfg Config) Validate() error {
	if cfg.FuturesSymbol == "" {
		return errors.New("a futures symbol is required")
	}
	if cfg.SpotSymbol == "" {
		return errors.New("a spot symbol is required")
	}
	if cfg.FlowQuantity == 0 {
		return errors.New("flow quantity must be positive")
	}
	if cfg.Interval <= 0 {
		return errors.New("interval must be positive")
	}
	if cfg.HedgeThreshold <= 0 {
		return errors.New("hedge threshold must be positive")
	}
	return nil
}
