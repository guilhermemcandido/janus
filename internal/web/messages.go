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
	Type   string       `json:"type"`
	Symbol string       `json:"symbol,omitempty"`
	Order  *orderView   `json:"order,omitempty"`
	Trades []*tradeView `json:"trades,omitempty"`
	Book   *bookView    `json:"book,omitempty"`
	Error  string       `json:"error,omitempty"`
}

type orderView struct {
	ID        uint64 `json:"id"`
	Remaining uint64 `json:"remaining"`
}

type tradeView struct {
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
	return &tradeView{Price: tr.Price, Quantity: tr.Quantity, MakerOrderID: tr.MakerOrderID, TakerOrderID: tr.TakerOrderID}
}

func tradeViewsFrom(trades []client.Trade) []*tradeView {
	views := make([]*tradeView, len(trades))
	for i, tr := range trades {
		views[i] = tradeViewFrom(tr)
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
