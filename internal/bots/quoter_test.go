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

func TestQuoter_ActRestsBidAndAsk(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	q := NewQuoter(c, testConfig(), fixedPriceSource{100})

	if err := q.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

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

func TestQuoter_ActReplacesPreviousQuotes(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	q := NewQuoter(c, testConfig(), fixedPriceSource{100})

	if err := q.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}
	firstBidID, firstAskID := q.bidID, q.askID

	if err := q.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

	if q.bidID == firstBidID || q.askID == firstAskID {
		t.Fatalf("expected fresh order IDs after second Act, got same IDs (%d, %d)", q.bidID, q.askID)
	}

	book, err := c.GetOrderBook(ctx, "AAPL", 10)
	if err != nil {
		t.Fatalf("GetOrderBook returned error: %v", err)
	}
	if len(book.Bids) != 1 || len(book.Asks) != 1 {
		t.Fatalf("book = %+v, want exactly one resting bid and ask (old quotes should be cancelled)", book)
	}
}

func TestQuoter_ActToleratesAlreadyFilledQuote(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	q := NewQuoter(c, testConfig(), fixedPriceSource{100})

	if err := q.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

	// Fill the bot's resting ask (100+2=102) from another participant.
	if _, _, err := c.SubmitOrder(ctx, "AAPL", client.Buy, client.Limit, 102, 10); err != nil {
		t.Fatalf("SubmitOrder returned error: %v", err)
	}

	var out bytes.Buffer
	if err := q.Act(ctx, &out); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

	if strings.Contains(out.String(), "error") {
		t.Fatalf("Act logged an error after a quote was already filled: %s", out.String())
	}
}

func TestQuoter_CancelIfRestingKeepsIDOnNonNotFoundError(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	q := NewQuoter(c, testConfig(), fixedPriceSource{100})

	if err := q.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}
	bidID := q.bidID

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	ok := q.cancelIfResting(cancelledCtx, &q.bidID, &bytes.Buffer{})

	if ok {
		t.Fatalf("cancelIfResting = true, want false (cancelled context should surface a non-NotFound error)")
	}
	if q.bidID != bidID {
		t.Fatalf("bidID = %d, want unchanged %d after a failed cancel", q.bidID, bidID)
	}
}

func TestQuoter_ActSkipsResubmitWhenCancelFails(t *testing.T) {
	c := newTestClient(t)
	q := NewQuoter(c, testConfig(), fixedPriceSource{100})

	if err := q.Act(context.Background(), &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}
	bidID := q.bidID

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := q.Act(cancelledCtx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

	if q.bidID != bidID {
		t.Fatalf("bidID = %d, want unchanged %d: Act must not resubmit when the old quote's cancel state is unknown", q.bidID, bidID)
	}
}

func TestQuoter_ActFailedTracksCancelError(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	q := NewQuoter(c, testConfig(), fixedPriceSource{100})

	if err := q.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}
	if q.ActFailed() {
		t.Fatalf("ActFailed() = true after a clean cycle, want false")
	}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := q.Act(cancelledCtx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}
	if !q.ActFailed() {
		t.Fatalf("ActFailed() = false after a cancel error, want true")
	}
}

func TestQuoter_ResetClearsOrderIDs(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	q := NewQuoter(c, testConfig(), fixedPriceSource{100})

	if err := q.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

	q.Reset(ctx, &bytes.Buffer{})

	if q.bidID != 0 || q.askID != 0 {
		t.Fatalf("bidID=%d askID=%d after Reset, want both 0", q.bidID, q.askID)
	}
}

func TestQuoter_CloseCancelsRestingQuotes(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	q := NewQuoter(c, testConfig(), fixedPriceSource{100})

	if err := q.Act(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Act returned error: %v", err)
	}

	var _ Closer = q // Quoter must satisfy Closer for Trader to clean it up on shutdown.
	q.Close(ctx, &bytes.Buffer{})

	book, err := c.GetOrderBook(ctx, "AAPL", 10)
	if err != nil {
		t.Fatalf("GetOrderBook returned error: %v", err)
	}
	if len(book.Bids) != 0 || len(book.Asks) != 0 {
		t.Fatalf("book = %+v, want empty after Close", book)
	}
}
