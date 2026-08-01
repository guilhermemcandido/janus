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
	"github.com/guilhermemcandido/janus/internal/bots/strategies/spot"
	"github.com/guilhermemcandido/janus/pkg/client"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC server address")
	qty := flag.Uint64("qty", 10, "quantity quoted on each side")
	halfSpread := flag.Int64("spread", 2, "distance in ticks from the reference price to each quote")
	interval := flag.Duration("interval", time.Second, "how often to requote")
	initialPrice := flag.Int64("initial-price", 100, "starting reference price, in ticks")
	walkStep := flag.Int64("walk-step", 1, "max ticks the reference price moves per requote")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `spot continuously quotes both sides of a Janus book around a random-walk reference price.

		Usage (flags must come before the symbol):
		spot [flags] <SYM>

		Flags:
		`)
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 || args[0] == "" {
		fmt.Fprintln(os.Stderr, "error: a symbol is required, e.g. `spot AAPL`")
		flag.Usage()
		os.Exit(2)
	}
	symbol := args[0]

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
	if *initialPrice <= 0 {
		fmt.Fprintln(os.Stderr, "error: initial price must be positive")
		os.Exit(2)
	}
	if *walkStep < 0 {
		fmt.Fprintln(os.Stderr, "error: walk step must not be negative")
		os.Exit(2)
	}

	c, err := client.Dial(*addr)
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer c.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	source := spot.NewRandomWalk(*initialPrice, *walkStep)
	trader := bots.NewTrader(bots.NewQuoter(c, cfg, source), cfg.Interval, c)
	if err := trader.Run(ctx, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
