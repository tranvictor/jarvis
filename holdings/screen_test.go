package holdings

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/ui"
)

func TestLiveScreenRewritesOnlyTheChangedLine(t *testing.T) {
	var buf bytes.Buffer
	s := &liveScreen{out: &buf, cursor: true, rows: 10, cols: 80}
	s.paint("header\nrow-a\nrow-b\n")
	buf.Reset()
	s.paint("header\nrow-a\nrow-B\n")

	got := buf.String()
	if strings.Contains(got, "header") || strings.Contains(got, "row-a") {
		t.Fatalf("unchanged lines were rewritten: %q", got)
	}
	if !strings.Contains(got, "\033[3;1Hrow-B\033[K") {
		t.Fatalf("changed line was not patched: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("patch advanced a row: %q", got)
	}
}

func TestLiveScreenErasesLinesThatWentAway(t *testing.T) {
	var buf bytes.Buffer
	s := &liveScreen{out: &buf, cursor: true, rows: 10, cols: 40}
	s.paint("a\nb\nc\n")
	buf.Reset()
	s.paint("a\n")

	got := buf.String()
	if strings.Contains(got, "a") {
		t.Fatalf("unchanged text returned: %q", got)
	}
	if !strings.Contains(got, "\033[2;1H\033[K") || !strings.Contains(got, "\033[3;1H\033[K") {
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
		Live:    true,
		Positions: []*Position{
			{ChainID: 1, ChainName: "Ethereum", TokenAddress: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Symbol: "ETH", Amount: "0.05", Value: &eth, Mark: "sell", ExitTier: "deep"},
			{ChainID: 1, ChainName: "Ethereum", TokenAddress: "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", Symbol: "USDC", Amount: "20", Value: &usdc, Mark: "sell", ExitTier: "deep"},
		},
	}
	book.recomputeTotals()

	var buf bytes.Buffer
	s := &liveScreen{out: &buf, cursor: true, rows: 40, cols: 120}
	s.paint(portfolioFrame(book))
	buf.Reset()

	eth = 101
	book.recomputeTotals()
	book.ValueTicks = map[string]int{book.Positions[0].Key(): 1}
	book.TotalTick = 1
	s.paint(portfolioFrame(book))

	got := buf.String()
	if strings.Contains(got, "USDC") {
		t.Fatalf("untouched USDC row was rewritten:\n%s", got)
	}
	if !strings.Contains(got, "101.00") {
		t.Fatalf("ETH value was not patched:\n%s", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("frame was printed from scratch:\n%s", got)
	}
}

func portfolioFrame(b *Book) string {
	var buf bytes.Buffer
	view := ui.NewTerminalUIWithWriter(&buf, false)
	view.LimitWidth(100)
	Render(view, b)
	return buf.String()
}

func TestLiveScreenPrintsWhenCursorCannotMove(t *testing.T) {
	var buf bytes.Buffer
	s := &liveScreen{out: &buf, cursor: false}
	s.paint("ETH  1.00\n")
	s.paint("ETH  1.10\n")
	if got, want := buf.String(), "ETH  1.00\nETH  1.10\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLiveScreenWrapsALineThatFillsTheWidth(t *testing.T) {
	var buf bytes.Buffer
	s := &liveScreen{out: &buf, cursor: true, rows: 6, cols: 10}
	s.paint(strings.Repeat("a", 10) + "\n")

	screen := applyMoves(t, nil, buf.String(), 6)
	if ui.VisibleWidth(screen[0]) > 9 || ui.VisibleWidth(screen[1]) == 0 {
		t.Fatalf("a full-width line skipped a row: %#v", screen)
	}
	if strings.TrimSpace(screen[2]) != "" {
		t.Fatalf("content landed past the wrap: %#v", screen)
	}
}

func TestLiveScreenKeepsATallFrameInsideTheTerminal(t *testing.T) {
	var lines []string
	lines = append(lines, "0xd8dA…6045    1,000.00 USD")
	for i := 0; i < 40; i++ {
		lines = append(lines, fmt.Sprintf("row-%02d", i))
	}
	lines = append(lines, "live · heartbeat 1s")

	var buf bytes.Buffer
	s := &liveScreen{out: &buf, cursor: true, rows: 8, cols: 40}
	s.paint(strings.Join(lines, "\n"))
	screen := applyMoves(t, nil, buf.String(), 8)
	if screen[0] != lines[0] {
		t.Fatalf("header moved: %#v", screen)
	}
	if !strings.Contains(screen[6], "lines hidden") {
		t.Fatalf("tall frame was not clipped: %#v", screen)
	}
	if screen[7] != "live · heartbeat 1s" {
		t.Fatalf("status line missing: %#v", screen)
	}
	for _, row := range screen {
		if ui.VisibleWidth(row) >= 40 {
			t.Fatalf("row is wide enough to wrap: %q", row)
		}
	}

	buf.Reset()
	lines[0] = "0xd8dA…6045    1,001.00 USD"
	prev := append([]string(nil), screen...)
	s.paint(strings.Join(lines, "\n"))
	next := applyMoves(t, screen, buf.String(), 8)
	if next[0] != lines[0] {
		t.Fatalf("header was not patched in place: %#v", next)
	}
	if next[1] != prev[1] || next[7] != prev[7] {
		t.Fatalf("untouched rows moved: %#v", next)
	}
	if strings.Count(strings.Join(next, "\n"), "0xd8dA") != 1 {
		t.Fatalf("header was printed into the table: %#v", next)
	}
}

func TestMoneyTextReservesTickWidth(t *testing.T) {
	plain, _ := moneyText(1234.5, 0, true)
	up, _ := moneyText(1234.5, 1, true)
	down, _ := moneyText(1234.5, -1, true)
	if ui.VisibleWidth(plain) != ui.VisibleWidth(up) || ui.VisibleWidth(plain) != ui.VisibleWidth(down) {
		t.Fatalf("tick changed the width: %q %q %q", plain, up, down)
	}
	static, _ := moneyText(1234.5, 0, false)
	if static != "1,234.50" {
		t.Fatalf("static money = %q", static)
	}
}

func applyMoves(t *testing.T, screen []string, paint string, rows int) []string {
	t.Helper()
	if strings.Contains(paint, "\n") {
		t.Fatalf("cursor paint contains a newline: %q", paint)
	}
	if len(screen) < rows {
		grown := make([]string, rows)
		copy(grown, screen)
		screen = grown
	}
	rest := paint
	for rest != "" {
		if !strings.HasPrefix(rest, "\033[") {
			t.Fatalf("paint is not a cursor move: %q", rest)
		}
		semi := strings.IndexByte(rest, ';')
		if semi < 0 || !strings.HasPrefix(rest[semi:], ";1H") {
			t.Fatalf("paint is not an absolute row: %q", rest)
		}
		row, err := strconv.Atoi(rest[2:semi])
		if err != nil || row < 1 || row > rows {
			t.Fatalf("row %q is outside the screen", rest[2:semi])
		}
		body := rest[semi+3:]
		end := strings.Index(body, "\033[K")
		if end < 0 {
			t.Fatalf("paint line was not cleared: %q", body)
		}
		screen[row-1] = body[:end]
		rest = body[end+3:]
	}
	return screen
}
