package cli

import (
	"testing"

	"github.com/guilhermemcandido/janus/pkg/client"
)

func TestParse_LimitBuy(t *testing.T) {
	cmd, err := Parse("buy 100 @ 50")
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}
	want := Command{Kind: Submit, Side: client.Buy, Type: client.Limit, Price: 50, Quantity: 100}
	if cmd != want {
		t.Fatalf("Parse(%q) = %+v, want %+v", "buy 100 @ 50", cmd, want)
	}
}

func TestParse_LimitSell(t *testing.T) {
	cmd, err := Parse("sell 50 @ 49")
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}
	want := Command{Kind: Submit, Side: client.Sell, Type: client.Limit, Price: 49, Quantity: 50}
	if cmd != want {
		t.Fatalf("Parse(%q) = %+v, want %+v", "sell 50 @ 49", cmd, want)
	}
}

func TestParse_MarketOrder(t *testing.T) {
	cmd, err := Parse("buy 10 market")
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}
	want := Command{Kind: Submit, Side: client.Buy, Type: client.Market, Quantity: 10}
	if cmd != want {
		t.Fatalf("Parse(%q) = %+v, want %+v", "buy 10 market", cmd, want)
	}
}

func TestParse_Cancel(t *testing.T) {
	cmd, err := Parse("cancel 42")
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}
	if cmd != (Command{Kind: Cancel, OrderID: 42}) {
		t.Fatalf("Parse(%q) = %+v, want Cancel(42)", "cancel 42", cmd)
	}
}

func TestParse_BookDefaultDepth(t *testing.T) {
	cmd, err := Parse("book")
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}
	if cmd != (Command{Kind: Book, Depth: 10}) {
		t.Fatalf("Parse(%q) = %+v, want Book(depth 10)", "book", cmd)
	}
}

func TestParse_BookExplicitDepth(t *testing.T) {
	cmd, err := Parse("book 5")
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}
	if cmd != (Command{Kind: Book, Depth: 5}) {
		t.Fatalf("Parse(%q) = %+v, want Book(depth 5)", "book 5", cmd)
	}
}

func TestParse_WatchHelpQuit(t *testing.T) {
	cases := map[string]Kind{
		"watch": Watch,
		"help":  Help,
		"quit":  Quit,
		"exit":  Quit,
	}
	for input, want := range cases {
		cmd, err := Parse(input)
		if err != nil {
			t.Fatalf("Parse(%q) returned unexpected error: %v", input, err)
		}
		if cmd.Kind != want {
			t.Fatalf("Parse(%q).Kind = %v, want %v", input, cmd.Kind, want)
		}
	}
}

func TestParse_BlankLineAndCommentAreNoop(t *testing.T) {
	for _, input := range []string{"", "   ", "# a comment"} {
		cmd, err := Parse(input)
		if err != nil {
			t.Fatalf("Parse(%q) returned unexpected error: %v", input, err)
		}
		if cmd.Kind != Noop {
			t.Fatalf("Parse(%q).Kind = %v, want Noop", input, cmd.Kind)
		}
	}
}

func TestParse_UnknownCommand(t *testing.T) {
	if _, err := Parse("frobnicate 5"); err == nil {
		t.Fatalf("expected an error for an unknown command")
	}
}

func TestParse_MalformedSubmit(t *testing.T) {
	cases := []string{"buy", "buy 100", "buy 100 at 50", "buy abc @ 50", "buy 100 @ abc"}
	for _, input := range cases {
		if _, err := Parse(input); err == nil {
			t.Fatalf("Parse(%q) expected an error, got none", input)
		}
	}
}

func TestParse_CancelRequiresID(t *testing.T) {
	cases := []string{"cancel", "cancel abc"}
	for _, input := range cases {
		if _, err := Parse(input); err == nil {
			t.Fatalf("Parse(%q) expected an error, got none", input)
		}
	}
}
