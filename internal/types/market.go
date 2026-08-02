package types

// MarketStats summarizes trading activity for one symbol since its engine started.
type MarketStats struct {
	Symbol    string
	HasTraded bool
	LastPrice int64
	OpenPrice int64
	High      int64
	Low       int64
	Volume    uint64
}
