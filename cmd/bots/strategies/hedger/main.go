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
	"github.com/guilhermemcandido/janus/internal/bots/strategies/hedger"
	"github.com/guilhermemcandido/janus/pkg/client"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC server address")
	futuresSymbol := flag.String("futures", "", "futures symbol to trade (required)")
	spotSymbol := flag.String("spot", "", "spot symbol to hedge with (required)")
	flowQty := flag.Uint64("flow-qty", 5, "quantity per random flow trade")
	interval := flag.Duration("interval", time.Second, "how often to consider a flow trade")
	threshold := flag.Int64("hedge-threshold", 20, "net position size that triggers a hedge")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `hedger takes random flow positions in a futures instrument and hedges net exposure via the underlying spot instrument.

Usage:
  hedger -futures <FUT_SYM> -spot <SPOT_SYM> [flags]

Flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	cfg := hedger.Config{
		FuturesSymbol:  *futuresSymbol,
		SpotSymbol:     *spotSymbol,
		FlowQuantity:   *flowQty,
		Interval:       *interval,
		HedgeThreshold: *threshold,
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

	trader := bots.NewTrader(hedger.New(c, cfg), cfg.Interval)
	if err := trader.Run(ctx, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
