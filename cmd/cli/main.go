package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/guilhermemcandido/janus/internal/cli"
	"github.com/guilhermemcandido/janus/pkg/client"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC server address")
	script := flag.String("script", "", "path to a script file of commands to replay (omit for interactive mode)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `janus is a gRPC client for the Janus exchange.

		Usage (flags must come before the symbol):
		janus [-addr <addr>] <SYM>                      interactive session
		janus [-addr <addr>] -script <file> <SYM>        replay a script file
		janus [-addr <addr>] <SYM> <command...>          run one command and exit

		Flags:
		`)
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		cli.PrintHelp(os.Stderr)
	}
	flag.Parse()

	args := flag.Args()
	var symbol string
	if len(args) > 0 {
		symbol, args = args[0], args[1:]
	}
	if err := cli.RequireSymbol(symbol); err != nil {
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

	if len(args) > 0 {
		cmd, err := cli.Parse(strings.Join(args, " "))
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(2)
		}
		cli.Exec(ctx, c, symbol, cmd, os.Stdout)
		return
	}

	if *script != "" {
		f, err := os.Open(*script)
		if err != nil {
			log.Fatalf("open script %s: %v", *script, err)
		}
		defer f.Close()

		if err := cli.Run(ctx, c, symbol, f, os.Stdout, false); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := cli.Run(ctx, c, symbol, os.Stdin, os.Stdout, true); err != nil {
		log.Fatal(err)
	}
}
