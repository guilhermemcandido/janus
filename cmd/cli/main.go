package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/guilhermemcandido/janus/internal/cli"
	"github.com/guilhermemcandido/janus/pkg/client"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC server address")
	symbol := flag.String("symbol", "", "trading symbol for this session (required)")
	script := flag.String("script", "", "path to a script file of commands to replay (omit for interactive mode)")
	flag.Parse()

	if err := cli.RequireSymbol(*symbol); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		flag.Usage()
		os.Exit(2)
	}

	c, err := client.Dial(*addr)
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer c.Close()

	ctx := context.Background()

	if *script != "" {
		f, err := os.Open(*script)
		if err != nil {
			log.Fatalf("open script %s: %v", *script, err)
		}
		defer f.Close()

		if err := cli.Run(ctx, c, *symbol, f, os.Stdout, false); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := cli.Run(ctx, c, *symbol, os.Stdin, os.Stdout, true); err != nil {
		log.Fatal(err)
	}
}
