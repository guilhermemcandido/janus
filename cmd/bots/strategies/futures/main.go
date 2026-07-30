package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/guilhermemcandido/janus/internal/bots"
	"github.com/guilhermemcandido/janus/internal/bots/strategies/futures"
	"github.com/guilhermemcandido/janus/pkg/client"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC server address")
	spotSymbol := flag.String("spot", "", "spot symbol this futures instrument is priced off of (required)")
	qty := flag.Uint64("qty", 10, "quantity quoted on each side")
	halfSpread := flag.Int64("spread", 2, "distance in ticks from the reference price to each quote")
	interval := flag.Duration("interval", time.Second, "how often to requote")
	basis := flag.Int64("basis", 0, "fixed offset added to the spot mid-price (futures = spot mid + basis)")
	fallback := flag.Int64("fallback-price", 100, "reference price to use until the spot book first has both a bid and an ask")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `futures continuously quotes both sides of a futures instrument's book, priced off a spot instrument's mid-price plus a fixed basis.

		Usage (flags must come before the symbol):
		futures -spot <SPOT_SYM> [flags] <FUT_SYM>

		Flags:
		`)
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 || args[0] == "" {
		fmt.Fprintln(os.Stderr, "error: a futures symbol is required, e.g. `futures -spot AAPL AAPLF`")
		flag.Usage()
		os.Exit(2)
	}
	symbol := args[0]

	if *spotSymbol == "" {
		fmt.Fprintln(os.Stderr, "error: -spot is required (which spot instrument should this be priced off of?)")
		flag.Usage()
		os.Exit(2)
	}

	cfg := bots.Config{
		Symbol:     symbol,
		Quantity:   *qty,
		HalfSpread: *halfSpread,
		Interval:   *interval,
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	if *fallback <= 0 {
		fmt.Fprintln(os.Stderr, "error: fallback price must be positive")
		os.Exit(2)
	}

	c, err := client.Dial(*addr)
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer c.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	source := futures.NewSpotMidPriceSource(c, *spotSymbol, *basis, *fallback)
	bot := bots.New(c, cfg, source)
	if err := bot.Run(ctx, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
