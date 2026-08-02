package web

import (
	"fmt"

	"github.com/guilhermemcandido/janus/pkg/client"
)

// ClientMessage is a command sent from the browser over the WebSocket.
type ClientMessage struct {
	Type      string `json:"type"`
	Symbol    string `json:"symbol,omitempty"`
	Side      string `json:"side,omitempty"`
	OrderType string `json:"orderType,omitempty"`
	Price     int64  `json:"price,omitempty"`
	Quantity  uint64 `json:"quantity,omitempty"`
	OrderID   uint64 `json:"orderId,omitempty"`
}

// ServerMessage is a message pushed from the bridge to the browser.
type ServerMessage struct {
	Type    string        `json:"type"`
	Symbol  string        `json:"symbol,omitempty"`
	Order   *orderView    `json:"order,omitempty"`
	Trades  []*tradeView  `json:"trades,omitempty"`
	Book    *bookView     `json:"book,omitempty"`
	Markets []*marketView `json:"markets,omitempty"`
	Error   string        `json:"error,omitempty"`
}

type orderView struct {
	ID        uint64 `json:"id"`
	Remaining uint64 `json:"remaining"`
}

type tradeView struct {
	ID           uint64 `json:"id"`
	Price        int64  `json:"price"`
	Quantity     uint64 `json:"quantity"`
	MakerOrderID uint64 `json:"makerOrderId"`
	TakerOrderID uint64 `json:"takerOrderId"`
}

type priceLevelView struct {
	Price    int64  `json:"price"`
	Quantity uint64 `json:"quantity"`
}

type bookView struct {
	Bids []priceLevelView `json:"bids"`
	Asks []priceLevelView `json:"asks"`
}

type marketView struct {
	Symbol      string          `json:"symbol"`
	Description string          `json:"description"`
	HasTraded   bool            `json:"hasTraded"`
	LastPrice   int64           `json:"lastPrice"`
	OpenPrice   int64           `json:"openPrice"`
	High        int64           `json:"high"`
	Low         int64           `json:"low"`
	Volume      uint64          `json:"volume"`
	BestBid     *priceLevelView `json:"bestBid,omitempty"`
	BestAsk     *priceLevelView `json:"bestAsk,omitempty"`
}

func parseSide(s string) (client.Side, error) {
	switch s {
	case "buy":
		return client.Buy, nil
	case "sell":
		return client.Sell, nil
	default:
		return 0, fmt.Errorf(`invalid side %q, want "buy" or "sell"`, s)
	}
}

func parseOrderType(s string) (client.OrderType, error) {
	switch s {
	case "limit":
		return client.Limit, nil
	case "market":
		return client.Market, nil
	default:
		return 0, fmt.Errorf(`invalid order type %q, want "limit" or "market"`, s)
	}
}

func tradeViewFrom(tr client.Trade) *tradeView {
	return &tradeView{ID: tr.ID, Price: tr.Price, Quantity: tr.Quantity, MakerOrderID: tr.MakerOrderID, TakerOrderID: tr.TakerOrderID}
}

func tradeViewsFrom(trades []client.Trade) []*tradeView {
	views := make([]*tradeView, len(trades))
	for i, tr := range trades {
		views[i] = tradeViewFrom(tr)
	}
	return views
}

func priceLevelViewFrom(l *client.PriceLevel) *priceLevelView {
	if l == nil {
		return nil
	}
	return &priceLevelView{Price: l.Price, Quantity: l.Quantity}
}

func marketViewFrom(m client.MarketSummary) *marketView {
	return &marketView{
		Symbol:      m.Symbol,
		Description: m.Description,
		HasTraded:   m.HasTraded,
		LastPrice:   m.LastPrice,
		OpenPrice:   m.OpenPrice,
		High:        m.High,
		Low:         m.Low,
		Volume:      m.Volume,
		BestBid:     priceLevelViewFrom(m.BestBid),
		BestAsk:     priceLevelViewFrom(m.BestAsk),
	}
}

func marketViewsFrom(markets []client.MarketSummary) []*marketView {
	views := make([]*marketView, len(markets))
	for i, m := range markets {
		views[i] = marketViewFrom(m)
	}
	return views
}

func bookViewFrom(book *client.BookSnapshot) *bookView {
	v := &bookView{Bids: make([]priceLevelView, len(book.Bids)), Asks: make([]priceLevelView, len(book.Asks))}
	for i, l := range book.Bids {
		v.Bids[i] = priceLevelView{Price: l.Price, Quantity: l.Quantity}
	}
	for i, l := range book.Asks {
		v.Asks[i] = priceLevelView{Price: l.Price, Quantity: l.Quantity}
	}
	return v
}
