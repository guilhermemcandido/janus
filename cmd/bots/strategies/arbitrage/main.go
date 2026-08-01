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
	"github.com/guilhermemcandido/janus/internal/bots/strategies/arbitrage"
	"github.com/guilhermemcandido/janus/pkg/client"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC server address")
	spotSymbol := flag.String("spot", "", "spot symbol (required)")
	futuresSymbol := flag.String("futures", "", "futures symbol (required)")
	fairBasis := flag.Int64("fair-basis", 5, "expected futures = spot mid + fair-basis")
	entryThreshold := flag.Int64("entry-threshold", 4, "spread deviation from fair basis that triggers a position")
	qty := flag.Uint64("qty", 10, "quantity per leg")
	interval := flag.Duration("interval", time.Second, "how often to check the spread")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `arbitrage trades the spot/futures spread against a fair basis, closing the position once the spread reverts.

Usage:
  arbitrage -spot <SPOT_SYM> -futures <FUT_SYM> [flags]

Flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	cfg := arbitrage.Config{
		SpotSymbol:     *spotSymbol,
		FuturesSymbol:  *futuresSymbol,
		FairBasis:      *fairBasis,
		EntryThreshold: *entryThreshold,
		Quantity:       *qty,
		Interval:       *interval,
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		flag.Usage()
		os.Exit(2)
	}

	c, err := client.Dial(*addr)
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer c.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	trader := bots.NewTrader(arbitrage.New(c, cfg), cfg.Interval, c)
	if err := trader.Run(ctx, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
