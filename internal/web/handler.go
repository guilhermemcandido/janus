package web

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/guilhermemcandido/janus/internal/websocket"
	"github.com/guilhermemcandido/janus/pkg/client"
)

// requestTimeout bounds each gRPC call triggered by a single incoming WebSocket message.
const requestTimeout = 5 * time.Second

// bookDepth is how many price levels per side are pushed with each book snapshot.
const bookDepth = 10

// HandleConn bridges one browser connection to c until the connection closes or errors.
func HandleConn(conn *websocket.Conn, c *client.Client) {
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := &connHandler{conn: conn, client: c, ctx: ctx, subs: make(map[string]context.CancelFunc)}
	defer h.stopAllSubscriptions()

	for {
		op, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if op != websocket.OpText {
			continue
		}
		h.handleMessage(payload)
	}
}

type connHandler struct {
	conn   *websocket.Conn
	client *client.Client
	ctx    context.Context

	mu   sync.Mutex
	subs map[string]context.CancelFunc // symbol -> cancel for its trade-forwarding goroutine
}

func (h *connHandler) handleMessage(payload []byte) {
	var msg ClientMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.sendError("invalid message: " + err.Error())
		return
	}

	switch msg.Type {
	case "submit":
		h.handleSubmit(msg)
	case "cancel":
		h.handleCancel(msg)
	case "subscribe":
		h.handleSubscribe(msg.Symbol)
	case "unsubscribe":
		h.handleUnsubscribe(msg.Symbol)
	default:
		h.sendError("unknown message type " + msg.Type)
	}
}

func (h *connHandler) handleSubmit(msg ClientMessage) {
	side, err := parseSide(msg.Side)
	if err != nil {
		h.sendError(err.Error())
		return
	}
	typ, err := parseOrderType(msg.OrderType)
	if err != nil {
		h.sendError(err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(h.ctx, requestTimeout)
	defer cancel()
	order, trades, err := h.client.SubmitOrder(ctx, msg.Symbol, side, typ, msg.Price, msg.Quantity)
	if err != nil {
		h.sendError(client.FriendlyError(err))
		return
	}

	h.send(ServerMessage{Type: "ack", Symbol: msg.Symbol, Order: &orderView{ID: order.ID, Remaining: order.Remaining}, Trades: tradeViewsFrom(trades)})
	if len(trades) > 0 {
		h.pushBook(msg.Symbol)
	}
}

func (h *connHandler) handleCancel(msg ClientMessage) {
	ctx, cancel := context.WithTimeout(h.ctx, requestTimeout)
	defer cancel()
	order, err := h.client.CancelOrder(ctx, msg.Symbol, msg.OrderID)
	if err != nil {
		h.sendError(client.FriendlyError(err))
		return
	}
	h.send(ServerMessage{Type: "ack", Symbol: msg.Symbol, Order: &orderView{ID: order.ID, Remaining: order.Remaining}})
	h.pushBook(msg.Symbol)
}

func (h *connHandler) handleSubscribe(symbol string) {
	if symbol == "" {
		h.sendError("subscribe requires a symbol")
		return
	}

	h.mu.Lock()
	if _, exists := h.subs[symbol]; exists {
		h.mu.Unlock()
		return
	}
	subCtx, cancel := context.WithCancel(h.ctx)
	h.subs[symbol] = cancel
	h.mu.Unlock()

	trades, err := h.client.SubscribeTrades(subCtx, symbol)
	if err != nil {
		cancel()
		h.mu.Lock()
		delete(h.subs, symbol)
		h.mu.Unlock()
		h.sendError("subscribe " + symbol + ": " + client.FriendlyError(err))
		return
	}

	h.pushBook(symbol)

	go func() {
		for tr := range trades {
			h.send(ServerMessage{Type: "trade", Symbol: symbol, Trades: []*tradeView{tradeViewFrom(tr)}})
			h.pushBook(symbol)
		}
	}()
}

func (h *connHandler) handleUnsubscribe(symbol string) {
	h.mu.Lock()
	cancel, ok := h.subs[symbol]
	if ok {
		delete(h.subs, symbol)
	}
	h.mu.Unlock()
	if ok {
		cancel()
	}
}

func (h *connHandler) stopAllSubscriptions() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for symbol, cancel := range h.subs {
		cancel()
		delete(h.subs, symbol)
	}
}

func (h *connHandler) pushBook(symbol string) {
	ctx, cancel := context.WithTimeout(h.ctx, requestTimeout)
	defer cancel()
	book, err := h.client.GetOrderBook(ctx, symbol, bookDepth)
	if err != nil {
		return // best-effort: a trade notification without a fresh book isn't fatal
	}
	h.send(ServerMessage{Type: "book", Symbol: symbol, Book: bookViewFrom(book)})
}

func (h *connHandler) send(msg ServerMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("web: marshal message: %v", err)
		return
	}
	_ = h.conn.WriteMessage(websocket.OpText, data) // a write error means the connection is gone; the read loop will notice
}

func (h *connHandler) sendError(msg string) {
	h.send(ServerMessage{Type: "error", Error: msg})
}
