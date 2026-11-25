package holdings

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/ui"
)

func TestLiveScreenRewritesOnlyTheChangedLine(t *testing.T) {
	var buf bytes.Buffer
	s := &liveScreen{out: &buf, cursor: true}
	s.paint("header\nrow-a\nrow-b\n", 80)
	buf.Reset()
	s.paint("header\nrow-a\nrow-B\n", 80)

	got := buf.String()
	if strings.Contains(got, "header") || strings.Contains(got, "row-a") {
		t.Fatalf("unchanged lines were rewritten: %q", got)
	}
	if !strings.Contains(got, "\033[3A") {
		t.Fatalf("cursor did not return to the frame: %q", got)
	}
	if !strings.Contains(got, "\rrow-B\033[K\n") {
		t.Fatalf("changed line was not patched: %q", got)
	}
}

func TestLiveScreenErasesLinesThatWentAway(t *testing.T) {
	var buf bytes.Buffer
	s := &liveScreen{out: &buf, cursor: true}
	s.paint("a\nb\nc\n", 40)
	buf.Reset()
	s.paint("a\n", 40)

	got := buf.String()
	if strings.Contains(got, "a\n") && !strings.HasPrefix(got, "\033[") {
		t.Fatalf("rewrote the whole frame: %q", got)
	}
	if strings.Contains(got, "\na") || strings.Contains(got, "row") {
		t.Fatalf("unchanged text returned: %q", got)
	}
	if !strings.Contains(got, "\r\033[K\n") || !strings.Contains(got, "\033[2A") {
		t.Fatalf("leftover rows were not cleared: %q", got)
	}
}

func TestLiveScreenPatchesOnlyTheMovedPortfolioRows(t *testing.T) {
	eth := 100.0
	usdc := 20.0
	book := &Book{
		Address: "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
		Name:    "vitalik.eth",
		MinUSD:  1,
		Positions: []*Position{
			{ChainID: 1, ChainName: "Ethereum", TokenAddress: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Symbol: "ETH", Amount: "0.05", Value: &eth, Mark: "sell", ExitTier: "deep"},
			{ChainID: 1, ChainName: "Ethereum", TokenAddress: "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", Symbol: "USDC", Amount: "20", Value: &usdc, Mark: "sell", ExitTier: "deep"},
		},
	}
	book.recomputeTotals()

	var buf bytes.Buffer
	s := &liveScreen{out: &buf, cursor: true}
	s.paint(portfolioFrame(book), 120)
	buf.Reset()

	eth = 101
	book.recomputeTotals()
	s.paint(portfolioFrame(book), 120)

	got := buf.String()
	if strings.Contains(got, "USDC") {
		t.Fatalf("untouched USDC row was rewritten:\n%s", got)
	}
	if !strings.Contains(got, "101.00") {
		t.Fatalf("ETH value was not patched:\n%s", got)
	}
	if !strings.HasPrefix(got, "\033[") {
		t.Fatalf("frame was printed from scratch:\n%s", got)
	}
}

func portfolioFrame(b *Book) string {
	var buf bytes.Buffer
	Render(ui.NewTerminalUIWithWriter(&buf, false), b)
	return buf.String()
}

func TestLiveScreenPrintsWhenCursorCannotMove(t *testing.T) {
	var buf bytes.Buffer
	s := &liveScreen{out: &buf, cursor: false}
	s.paint("ETH  1.00\n", 80)
	s.paint("ETH  1.10\n", 80)
	if got, want := buf.String(), "ETH  1.00\nETH  1.10\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLiveScreenPrintsWhenALineDoesNotFit(t *testing.T) {
	var buf bytes.Buffer
	s := &liveScreen{out: &buf, cursor: true}
	s.paint("hi\n", 80)
	buf.Reset()
	s.paint("hello!!!\n", 4)

	got := buf.String()
	if strings.Contains(got, "\033[1A") {
		t.Fatalf("used in-place motion for a wrapped line: %q", got)
	}
	if !strings.Contains(got, "\033[H\033[2J") || !strings.Contains(got, "hello!!!\n") {
		t.Fatalf("did not fall back to a full reprint: %q", got)
	}
}
