package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/guilhermemcandido/janus/pkg/client"
)

type Kind int

const (
	Noop Kind = iota
	Submit
	Cancel
	Book
	Watch
	Help
	Quit
)

// Command is one parsed line of CLI input.
type Command struct {
	Kind     Kind
	Side     client.Side
	Type     client.OrderType
	Price    int64
	Quantity uint64
	OrderID  uint64
	Depth    int
}

// Parse turns one line of input into a Command.
func Parse(line string) (Command, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
		return Command{Kind: Noop}, nil
	}

	switch strings.ToLower(fields[0]) {
	case "buy", "sell":
		return parseSubmit(fields)
	case "cancel":
		return parseCancel(fields)
	case "book":
		return parseBook(fields)
	case "watch":
		return Command{Kind: Watch}, nil
	case "help":
		return Command{Kind: Help}, nil
	case "quit", "exit":
		return Command{Kind: Quit}, nil
	default:
		return Command{}, fmt.Errorf("unknown command %q (try 'help')", fields[0])
	}
}

func parseSubmit(fields []string) (Command, error) {
	side := client.Buy
	if strings.ToLower(fields[0]) == "sell" {
		side = client.Sell
	}

	if len(fields) < 2 {
		return Command{}, fmt.Errorf("usage: %s <qty> @ <price> | %s <qty> market", fields[0], fields[0])
	}
	qty, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return Command{}, fmt.Errorf("invalid quantity %q: %w", fields[1], err)
	}

	if len(fields) >= 3 && strings.ToLower(fields[2]) == "market" {
		return Command{Kind: Submit, Side: side, Type: client.Market, Quantity: qty}, nil
	}

	if len(fields) != 4 || fields[2] != "@" {
		return Command{}, fmt.Errorf("usage: %s <qty> @ <price> | %s <qty> market", fields[0], fields[0])
	}
	price, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return Command{}, fmt.Errorf("invalid price %q: %w", fields[3], err)
	}

	return Command{Kind: Submit, Side: side, Type: client.Limit, Price: price, Quantity: qty}, nil
}

func parseCancel(fields []string) (Command, error) {
	if len(fields) != 2 {
		return Command{}, fmt.Errorf("usage: cancel <order_id>")
	}
	id, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return Command{}, fmt.Errorf("invalid order id %q: %w", fields[1], err)
	}
	return Command{Kind: Cancel, OrderID: id}, nil
}

func parseBook(fields []string) (Command, error) {
	depth := 10
	if len(fields) >= 2 {
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			return Command{}, fmt.Errorf("invalid depth %q: %w", fields[1], err)
		}
		if n < 0 {
			return Command{}, fmt.Errorf("invalid depth %q: must not be negative", fields[1])
		}
		depth = n
	}
	return Command{Kind: Book, Depth: depth}, nil
}
