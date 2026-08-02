package cli

import (
	"fmt"
	"io"

	"github.com/guilhermemcandido/janus/pkg/client"
)

func formatOrder(o *client.Order) string {
	side := "BUY"
	if o.Side == client.Sell {
		side = "SELL"
	}
	typ := "LIMIT"
	if o.Type == client.Market {
		typ = "MARKET"
	}
	return fmt.Sprintf("%s %s %d @ %d (remaining %d)", side, typ, o.Quantity, o.Price, o.Remaining)
}

func printBook(out io.Writer, book *client.BookSnapshot) {
	fmt.Fprintf(out, "%-20s%-20s\n", "BIDS", "ASKS")
	n := len(book.Bids)
	if len(book.Asks) > n {
		n = len(book.Asks)
	}
	for i := 0; i < n; i++ {
		var bidStr, askStr string
		if i < len(book.Bids) {
			bidStr = fmt.Sprintf("%d x %d", book.Bids[i].Price, book.Bids[i].Quantity)
		}
		if i < len(book.Asks) {
			askStr = fmt.Sprintf("%d x %d", book.Asks[i].Price, book.Asks[i].Quantity)
		}
		fmt.Fprintf(out, "%-20s%-20s\n", bidStr, askStr)
	}
}

// PrintHelp writes the command grammar to out.
func PrintHelp(out io.Writer) {
	fmt.Fprintln(out, `Commands:
  register <description>  list this symbol on the exchange, so it can be traded
  buy <qty> @ <price>     submit a limit buy order
  sell <qty> @ <price>    submit a limit sell order
  buy <qty> market        submit a market buy order
  sell <qty> market       submit a market sell order
  cancel <order_id>       cancel a resting order
  book [depth]            show the order book (default depth 10)
  watch                   stream live trades until interrupted (Ctrl+C)
  help                    show this message
  quit, exit              exit`)
}
