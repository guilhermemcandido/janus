package web

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/guilhermemcandido/janus/internal/api"
	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/engine"
	"github.com/guilhermemcandido/janus/pkg/client"
)

const bufSize = 1024 * 1024

func newTestClient(t *testing.T) *client.Client {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	exchange := engine.NewExchange()
	pb.RegisterExchangeServer(grpcServer, api.NewServer(exchange))

	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		exchange.Close()
	})

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.Dial()
	}
	c, err := client.Dial("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	return c
}

// wsTestClient is a minimal raw WebSocket client, standing in for a browser.
type wsTestClient struct {
	t    *testing.T
	conn net.Conn
	br   *bufio.Reader
}

func dialWS(t *testing.T, addr string) *wsTestClient {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	req := fmt.Sprintf("GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n", addr)
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil || !strings.Contains(status, "101") {
		t.Fatalf("handshake failed: status=%q err=%v", status, err)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("read header: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}
	return &wsTestClient{t: t, conn: conn, br: br}
}

func (c *wsTestClient) sendJSON(v any) {
	c.t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		c.t.Fatalf("marshal: %v", err)
	}

	key := [4]byte{0x01, 0x02, 0x03, 0x04}
	masked := append([]byte(nil), data...)
	for i := range masked {
		masked[i] ^= key[i%4]
	}

	header := []byte{0x81}
	n := len(masked)
	if n <= 125 {
		header = append(header, 0x80|byte(n))
	} else {
		header = append(header, 0x80|126)
		ext := make([]byte, 2)
		binary.BigEndian.PutUint16(ext, uint16(n))
		header = append(header, ext...)
	}
	header = append(header, key[:]...)
	if _, err := c.conn.Write(append(header, masked...)); err != nil {
		c.t.Fatalf("write frame: %v", err)
	}
}

// readServerFrame reads one frame; server frames are never masked, so no unmasking is needed.
func (c *wsTestClient) readServerFrame() (byte, []byte) {
	c.t.Helper()
	head := make([]byte, 2)
	if _, err := io.ReadFull(c.br, head); err != nil {
		c.t.Fatalf("read frame header: %v", err)
	}
	opcode := head[0] & 0x0F
	length := int(head[1] & 0x7F)
	switch length {
	case 126:
		ext := make([]byte, 2)
		io.ReadFull(c.br, ext)
		length = int(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		io.ReadFull(c.br, ext)
		length = int(binary.BigEndian.Uint64(ext))
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		c.t.Fatalf("read frame payload: %v", err)
	}
	return opcode, payload
}

func (c *wsTestClient) readMessage() ServerMessage {
	c.t.Helper()
	_, payload := c.readServerFrame()
	var msg ServerMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		c.t.Fatalf("unmarshal %s: %v", payload, err)
	}
	return msg
}

func newTestServer(t *testing.T, c *client.Client) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(NewWebSocketHandler(c))
	t.Cleanup(srv.Close)
	return srv
}

func registerMarket(t *testing.T, c *client.Client, symbol, description string) {
	t.Helper()
	if _, err := c.RegisterMarket(context.Background(), symbol, description); err != nil {
		t.Fatalf("RegisterMarket(%q) returned error: %v", symbol, err)
	}
}

func serverAddr(srv *httptest.Server) string {
	return srv.Listener.Addr().String()
}

func TestHandleConn_SubmitRestingOrderReturnsAck(t *testing.T) {
	c := newTestClient(t)
	srv := newTestServer(t, c)
	registerMarket(t, c, "AAPL", "Apple Inc.")
	ws := dialWS(t, serverAddr(srv))

	ws.sendJSON(ClientMessage{Type: "submit", Symbol: "AAPL", Side: "buy", OrderType: "limit", Price: 100, Quantity: 10})

	msg := ws.readMessage()
	if msg.Type != "ack" || msg.Order == nil || msg.Order.Remaining != 10 {
		t.Fatalf("msg = %+v, want ack with Remaining=10", msg)
	}
}

func TestHandleConn_SubmitCrossingOrderReturnsAckWithTradesAndBook(t *testing.T) {
	c := newTestClient(t)
	srv := newTestServer(t, c)
	registerMarket(t, c, "AAPL", "Apple Inc.")
	ws := dialWS(t, serverAddr(srv))

	ws.sendJSON(ClientMessage{Type: "submit", Symbol: "AAPL", Side: "sell", OrderType: "limit", Price: 100, Quantity: 10})
	if msg := ws.readMessage(); msg.Type != "ack" {
		t.Fatalf("first ack: got %+v", msg)
	}

	ws.sendJSON(ClientMessage{Type: "submit", Symbol: "AAPL", Side: "buy", OrderType: "limit", Price: 100, Quantity: 10})

	ack := ws.readMessage()
	if ack.Type != "ack" || len(ack.Trades) != 1 || ack.Trades[0].Quantity != 10 {
		t.Fatalf("second ack = %+v, want ack with one trade of quantity 10", ack)
	}
	bookMsg := ws.readMessage()
	if bookMsg.Type != "book" || len(bookMsg.Book.Bids) != 0 || len(bookMsg.Book.Asks) != 0 {
		t.Fatalf("book msg = %+v, want an empty book after the full cross", bookMsg)
	}
}

func TestHandleConn_SubmitInvalidSideReturnsError(t *testing.T) {
	c := newTestClient(t)
	srv := newTestServer(t, c)
	registerMarket(t, c, "AAPL", "Apple Inc.")
	ws := dialWS(t, serverAddr(srv))

	ws.sendJSON(ClientMessage{Type: "submit", Symbol: "AAPL", Side: "sideways", OrderType: "limit", Price: 100, Quantity: 10})

	msg := ws.readMessage()
	if msg.Type != "error" || msg.Error == "" {
		t.Fatalf("msg = %+v, want a non-empty error", msg)
	}
}

func TestHandleConn_CancelReturnsAck(t *testing.T) {
	c := newTestClient(t)
	srv := newTestServer(t, c)
	registerMarket(t, c, "AAPL", "Apple Inc.")
	ws := dialWS(t, serverAddr(srv))

	ws.sendJSON(ClientMessage{Type: "submit", Symbol: "AAPL", Side: "buy", OrderType: "limit", Price: 100, Quantity: 10})
	ack := ws.readMessage()

	ws.sendJSON(ClientMessage{Type: "cancel", Symbol: "AAPL", OrderID: ack.Order.ID})

	cancelAck := ws.readMessage()
	if cancelAck.Type != "ack" || cancelAck.Order.ID != ack.Order.ID {
		t.Fatalf("cancelAck = %+v, want ack for order %d", cancelAck, ack.Order.ID)
	}
	bookMsg := ws.readMessage()
	if bookMsg.Type != "book" || len(bookMsg.Book.Bids) != 0 {
		t.Fatalf("book msg = %+v, want an empty book after cancel", bookMsg)
	}
}

func TestHandleConn_SubscribeReceivesInitialBookThenTradeUpdates(t *testing.T) {
	c := newTestClient(t)
	srv := newTestServer(t, c)
	registerMarket(t, c, "AAPL", "Apple Inc.")
	ws := dialWS(t, serverAddr(srv))

	ws.sendJSON(ClientMessage{Type: "subscribe", Symbol: "AAPL"})
	history := ws.readMessage()
	if history.Type != "history" || len(history.Trades) != 0 {
		t.Fatalf("history = %+v, want empty", history)
	}
	initial := ws.readMessage()
	if initial.Type != "book" || len(initial.Book.Bids) != 0 || len(initial.Book.Asks) != 0 {
		t.Fatalf("initial book = %+v, want empty", initial)
	}

	if _, _, err := c.SubmitOrder(context.Background(), "AAPL", client.Sell, client.Limit, 100, 10); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}
	if _, _, err := c.SubmitOrder(context.Background(), "AAPL", client.Buy, client.Limit, 100, 10); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	tradeMsg := ws.readMessage()
	if tradeMsg.Type != "trade" || len(tradeMsg.Trades) != 1 || tradeMsg.Trades[0].Quantity != 10 {
		t.Fatalf("trade msg = %+v, want a single trade of quantity 10", tradeMsg)
	}
	bookMsg := ws.readMessage()
	if bookMsg.Type != "book" {
		t.Fatalf("book msg = %+v, want a book update", bookMsg)
	}
}

// Guards against a real bug found live: a self-triggered fill was sent both as its own submit's
// trade message and again via the subscription broadcast, doubling every self-fill on a tape UI.
func TestHandleConn_SubmitWhileSubscribedDeliversTradeExactlyOnce(t *testing.T) {
	c := newTestClient(t)
	srv := newTestServer(t, c)
	registerMarket(t, c, "AAPL", "Apple Inc.")
	ws := dialWS(t, serverAddr(srv))

	ws.sendJSON(ClientMessage{Type: "subscribe", Symbol: "AAPL"})
	ws.readMessage() // history backlog
	ws.readMessage() // initial book

	ws.sendJSON(ClientMessage{Type: "submit", Symbol: "AAPL", Side: "sell", OrderType: "limit", Price: 100, Quantity: 10})
	if msg := ws.readMessage(); msg.Type != "ack" {
		t.Fatalf("resting-order ack: got %+v", msg)
	}

	ws.sendJSON(ClientMessage{Type: "submit", Symbol: "AAPL", Side: "buy", OrderType: "limit", Price: 100, Quantity: 10})

	// The crossing submit's own ack and the subscription's independent broadcast of the same
	// trade arrive on different goroutines - count by type instead of assuming an exact order.
	var acks, tradeBroadcasts, tradesInAck int
	for i := 0; i < 4; i++ {
		switch msg := ws.readMessage(); msg.Type {
		case "ack":
			acks++
			tradesInAck += len(msg.Trades)
		case "trade":
			tradeBroadcasts++
		case "book":
		default:
			t.Fatalf("unexpected message type %q", msg.Type)
		}
	}

	if acks != 1 || tradesInAck != 1 {
		t.Fatalf("acks=%d tradesInAck=%d, want exactly one ack carrying exactly one trade", acks, tradesInAck)
	}
	if tradeBroadcasts != 1 {
		t.Fatalf("tradeBroadcasts=%d, want exactly one live trade broadcast, not a duplicate of the self-triggered fill", tradeBroadcasts)
	}
}

func TestHandleConn_SubscribeSeedsHistoryBacklog(t *testing.T) {
	c := newTestClient(t)
	srv := newTestServer(t, c)
	registerMarket(t, c, "AAPL", "Apple Inc.")

	if _, _, err := c.SubmitOrder(context.Background(), "AAPL", client.Sell, client.Limit, 100, 10); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}
	if _, _, err := c.SubmitOrder(context.Background(), "AAPL", client.Buy, client.Limit, 100, 10); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	ws := dialWS(t, serverAddr(srv))
	ws.sendJSON(ClientMessage{Type: "subscribe", Symbol: "AAPL"})

	history := ws.readMessage()
	if history.Type != "history" || len(history.Trades) != 1 || history.Trades[0].Quantity != 10 {
		t.Fatalf("history = %+v, want one backlog trade of quantity 10", history)
	}
}

func TestHandleConn_ListSymbolsReturnsMarketSummaries(t *testing.T) {
	c := newTestClient(t)
	srv := newTestServer(t, c)
	registerMarket(t, c, "AAPL", "Apple Inc.")

	if _, _, err := c.SubmitOrder(context.Background(), "AAPL", client.Sell, client.Limit, 100, 10); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}
	if _, _, err := c.SubmitOrder(context.Background(), "AAPL", client.Buy, client.Limit, 100, 10); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	ws := dialWS(t, serverAddr(srv))
	ws.sendJSON(ClientMessage{Type: "list_symbols"})

	msg := ws.readMessage()
	if msg.Type != "markets" || len(msg.Markets) != 1 {
		t.Fatalf("msg = %+v, want one market summary", msg)
	}
	m := msg.Markets[0]
	if m.Symbol != "AAPL" || !m.HasTraded || m.LastPrice != 100 || m.Volume != 10 {
		t.Fatalf("market = %+v, want AAPL traded at 100 with volume 10", m)
	}
}

func TestHandleConn_UnsubscribeStopsTradeForwarding(t *testing.T) {
	c := newTestClient(t)
	srv := newTestServer(t, c)
	registerMarket(t, c, "AAPL", "Apple Inc.")
	ws := dialWS(t, serverAddr(srv))

	ws.sendJSON(ClientMessage{Type: "subscribe", Symbol: "AAPL"})
	ws.readMessage() // history backlog
	ws.readMessage() // initial book

	ws.sendJSON(ClientMessage{Type: "unsubscribe", Symbol: "AAPL"})

	if _, _, err := c.SubmitOrder(context.Background(), "AAPL", client.Sell, client.Limit, 100, 10); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}
	if _, _, err := c.SubmitOrder(context.Background(), "AAPL", client.Buy, client.Limit, 100, 10); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	ws.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var netErr net.Error
	if _, err := ws.conn.Read(make([]byte, 1)); err == nil || !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("Read returned (%v), want a read-deadline timeout (no frame should arrive after unsubscribe)", err)
	}
}
