package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/guilhermemcandido/janus/internal/bots"
	"github.com/guilhermemcandido/janus/internal/bots/strategies/noise"
	"github.com/guilhermemcandido/janus/pkg/client"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC server address")
	symbols := flag.String("symbols", "", "comma-separated symbols to trade randomly (required)")
	qty := flag.Uint64("qty", 5, "quantity per trade")
	interval := flag.Duration("interval", time.Second, "how often to consider a trade")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `noise submits random-direction market orders on random symbols, adding uninformed volume with no view.

Usage:
  noise -symbols <SYM1,SYM2,...> [flags]

Flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *symbols == "" {
		fmt.Fprintln(os.Stderr, "error: -symbols is required, e.g. `noise -symbols AAPL,AAPLF`")
		flag.Usage()
		os.Exit(2)
	}

	cfg := noise.Config{
		Symbols:  strings.Split(*symbols, ","),
		Quantity: *qty,
		Interval: *interval,
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

	trader := bots.NewTrader(noise.New(c, cfg), cfg.Interval, c)
	if err := trader.Run(ctx, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
