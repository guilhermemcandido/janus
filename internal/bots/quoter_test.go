package bots

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

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

	go func() {
		_ = grpcServer.Serve(lis)
	}()
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

func testConfig() Config {
	return Config{
		Symbol:     "AAPL",
		Quantity:   10,
		HalfSpread: 2,
	}
}

// fixedPriceSource is a deterministic PriceSource for tests, independent of any concrete bot's own source.
type fixedPriceSource struct{ price int64 }

func (f fixedPriceSource) Price(ctx context.Context) (int64, error) {
	return f.price, nil
}

func TestBot_RequoteRestsBidAndAsk(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	bot := New(c, testConfig(), fixedPriceSource{100})

	bot.requote(ctx, &bytes.Buffer{})

	book, err := c.GetOrderBook(ctx, "AAPL", 10)
	if err != nil {
		t.Fatalf("GetOrderBook returned error: %v", err)
	}
	if len(book.Bids) != 1 || book.Bids[0].Price != 98 || book.Bids[0].Quantity != 10 {
		t.Fatalf("Bids = %+v, want [{98 10}]", book.Bids)
	}
	if len(book.Asks) != 1 || book.Asks[0].Price != 102 || book.Asks[0].Quantity != 10 {
		t.Fatalf("Asks = %+v, want [{102 10}]", book.Asks)
	}
}

func TestBot_RequoteReplacesPreviousQuotes(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	bot := New(c, testConfig(), fixedPriceSource{100})

	bot.requote(ctx, &bytes.Buffer{})
	firstBidID, firstAskID := bot.bidID, bot.askID

	bot.requote(ctx, &bytes.Buffer{})

	if bot.bidID == firstBidID || bot.askID == firstAskID {
		t.Fatalf("expected fresh order IDs after second requote, got same IDs (%d, %d)", bot.bidID, bot.askID)
	}

	book, err := c.GetOrderBook(ctx, "AAPL", 10)
	if err != nil {
		t.Fatalf("GetOrderBook returned error: %v", err)
	}
	if len(book.Bids) != 1 || len(book.Asks) != 1 {
		t.Fatalf("book = %+v, want exactly one resting bid and ask (old quotes should be cancelled)", book)
	}
}

func TestBot_RequoteToleratesAlreadyFilledQuote(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	bot := New(c, testConfig(), fixedPriceSource{100})

	bot.requote(ctx, &bytes.Buffer{})

	// Fill the bot's resting ask (100+2=102) from another participant.
	if _, _, err := c.SubmitOrder(ctx, "AAPL", client.Buy, client.Limit, 102, 10); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	var out bytes.Buffer
	bot.requote(ctx, &out)

	if strings.Contains(out.String(), "error") {
		t.Fatalf("requote logged an error after a quote was already filled: %s", out.String())
	}
}
