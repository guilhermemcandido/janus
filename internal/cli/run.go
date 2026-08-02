package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/guilhermemcandido/janus/pkg/client"
)

// Run reads commands from in, one per line, executes them against c, and writes results to out.
// If interactive is true, a prompt is printed before each read.
func Run(ctx context.Context, c *client.Client, symbol string, in io.Reader, out io.Writer, interactive bool) error {
	scanner := bufio.NewScanner(in)
	for {
		if interactive {
			fmt.Fprintf(out, "%s> ", symbol)
		}
		if !scanner.Scan() {
			return scanner.Err()
		}

		cmd, err := Parse(scanner.Text())
		if err != nil {
			fmt.Fprintln(out, "error:", err)
			continue
		}

		if quit := Exec(ctx, c, symbol, cmd, out); quit {
			return nil
		}
	}
}

// Exec executes a single parsed command against c, writing results to out.
// It reports whether cmd was Quit, so callers looping over multiple commands know to stop.
func Exec(ctx context.Context, c *client.Client, symbol string, cmd Command, out io.Writer) bool {
	switch cmd.Kind {
	case Quit:
		return true
	case Help:
		PrintHelp(out)
	case Submit:
		execSubmit(ctx, c, symbol, cmd, out)
	case Cancel:
		execCancel(ctx, c, symbol, cmd, out)
	case Book:
		execBook(ctx, c, symbol, cmd, out)
	case Watch:
		execWatch(ctx, c, symbol, out)
	case Register:
		execRegister(ctx, c, symbol, cmd, out)
	}
	return false
}

func execSubmit(ctx context.Context, c *client.Client, symbol string, cmd Command, out io.Writer) {
	order, trades, err := c.SubmitOrder(ctx, symbol, cmd.Side, cmd.Type, cmd.Price, cmd.Quantity)
	if err != nil {
		fmt.Fprintln(out, "error:", err)
		return
	}
	fmt.Fprintf(out, "order %d: %s\n", order.ID, formatOrder(order))
	for _, tr := range trades {
		fmt.Fprintf(out, "  matched %d @ %d (maker %d)\n", tr.Quantity, tr.Price, tr.MakerOrderID)
	}
}

func execCancel(ctx context.Context, c *client.Client, symbol string, cmd Command, out io.Writer) {
	order, err := c.CancelOrder(ctx, symbol, cmd.OrderID)
	if err != nil {
		fmt.Fprintln(out, "error:", err)
		return
	}
	fmt.Fprintf(out, "cancelled order %d (%d unfilled)\n", order.ID, order.Remaining)
}

func execBook(ctx context.Context, c *client.Client, symbol string, cmd Command, out io.Writer) {
	book, err := c.GetOrderBook(ctx, symbol, cmd.Depth)
	if err != nil {
		fmt.Fprintln(out, "error:", err)
		return
	}
	printBook(out, book)
}

func execRegister(ctx context.Context, c *client.Client, symbol string, cmd Command, out io.Writer) {
	market, err := c.RegisterMarket(ctx, symbol, cmd.Description)
	if err != nil {
		fmt.Fprintln(out, "error:", err)
		return
	}
	fmt.Fprintf(out, "registered %s: %s\n", market.Symbol, market.Description)
}

func execWatch(ctx context.Context, c *client.Client, symbol string, out io.Writer) {
	watchCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	trades, err := c.SubscribeTrades(watchCtx, symbol)
	if err != nil {
		fmt.Fprintln(out, "error:", err)
		return
	}
	fmt.Fprintln(out, "watching trades on", symbol, "(Ctrl+C to stop watching)")
	for tr := range trades {
		fmt.Fprintf(out, "trade: %d @ %d (maker %d, taker %d)\n", tr.Quantity, tr.Price, tr.MakerOrderID, tr.TakerOrderID)
	}
	fmt.Fprintln(out, "stopped watching")
}
