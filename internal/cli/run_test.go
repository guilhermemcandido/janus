package cli

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
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

	c, err := client.Dial("passthrough:///bufnet", grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	return c
}

func TestRun_SubmitMatchAndBook(t *testing.T) {
	c := newTestClient(t)

	script := strings.NewReader(strings.Join([]string{
		"register Apple Inc.",
		"sell 50 @ 100",
		"buy 20 @ 100",
		"book",
		"quit",
	}, "\n"))

	var out bytes.Buffer
	if err := Run(context.Background(), c, "AAPL", script, &out, false); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "order 1: SELL LIMIT 50 @ 100") {
		t.Fatalf("output missing sell confirmation, got:\n%s", got)
	}
	if !strings.Contains(got, "matched 20 @ 100 (maker 1)") {
		t.Fatalf("output missing match line, got:\n%s", got)
	}
	if !strings.Contains(got, "100 x 30") {
		t.Fatalf("output missing remaining ask depth, got:\n%s", got)
	}
}

func TestRun_CancelOrder(t *testing.T) {
	c := newTestClient(t)

	script := strings.NewReader(strings.Join([]string{
		"register Apple Inc.",
		"sell 50 @ 100",
		"cancel 1",
		"quit",
	}, "\n"))

	var out bytes.Buffer
	if err := Run(context.Background(), c, "AAPL", script, &out, false); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	if !strings.Contains(out.String(), "cancelled order 1 (50 unfilled)") {
		t.Fatalf("output missing cancel confirmation, got:\n%s", out.String())
	}
}

func TestRun_UnknownCommandReportsErrorAndContinues(t *testing.T) {
	c := newTestClient(t)

	script := strings.NewReader(strings.Join([]string{
		"register Apple Inc.",
		"frobnicate",
		"book",
		"quit",
	}, "\n"))

	var out bytes.Buffer
	if err := Run(context.Background(), c, "AAPL", script, &out, false); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "error:") {
		t.Fatalf("expected an error line for the unknown command, got:\n%s", got)
	}
	if !strings.Contains(got, "BIDS") {
		t.Fatalf("expected Run to continue past the bad command and still print the book, got:\n%s", got)
	}
}
