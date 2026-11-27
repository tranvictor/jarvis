package holdings

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/tranvictor/jarvis/ui"
)

func TestValueColumnRightAligns(t *testing.T) {
	eth := 1234.56
	usdc := 12345.0
	dai := 9.5
	up := 1.2
	down := -4.2
	book := &Book{
		Address:    "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
		Name:       "vitalik.eth",
		MinUSD:     1,
		Priced:     3,
		ChainCount: 2,
		Total:      eth + usdc + dai,
		Live:       true,
		Positions: []*Position{
			{ChainID: 1, ChainName: "Ethereum", TokenAddress: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Symbol: "ETH", Amount: "0.42", Value: &eth, Mark: "sell", SellUSD: "2940.5", ExitTier: "deep", Change24h: &up},
			{ChainID: 1, ChainName: "Ethereum", TokenAddress: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Symbol: "USDC", Amount: "12345", Value: &usdc, Mark: "buy", BuyUSD: "1.0001", ExitTier: "deep", Change24h: &up},
			{ChainID: 56, ChainName: "BSC", TokenAddress: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Symbol: "DAI", Amount: "9.5", Value: &dai, Mark: "settle", SettlementUSD: "0.9998", ExitTier: "thin", Change24h: &down},
			{ChainID: 56, ChainName: "BSC", TokenAddress: "0xcccccccccccccccccccccccccccccccccccccccc", Symbol: "DUST", Amount: "1", Value: nil},
		},
	}
	plain := renderPlain(t, book, false)
	values := valueColumn(t, plain)
	// header, ETH, USDC, DAI, unpriced dash
	if len(values) != 5 {
		t.Fatalf("value cells = %d, want 5\n%s", len(values), plain)
	}
	want := []string{"value", "▲ 1,234.56", "▲ 12,345.00", "▼ 9.50", "—"}
	for i, cell := range values {
		if !strings.HasSuffix(strings.TrimRight(cell, " "), want[i]) {
			t.Fatalf("cell %d = %q, want suffix %q\n%s", i, cell, want[i], plain)
		}
		if !cellRightAligned(cell) {
			t.Fatalf("cell %d is not flush right: %q\n%s", i, cell, plain)
		}
	}
	// A repeated chain name is blanked, and the amount column stays left.
	if !strings.Contains(plain, "USDC") || !strings.Contains(plain, "12,345") {
		t.Fatalf("row content missing:\n%s", plain)
	}
	prices := columnAt(t, plain, 5)
	priceWant := []string{"price", "▲ 2,940.5", "▲ 1.0001", "▼ 0.9998", "—"}
	if len(prices) != len(priceWant) {
		t.Fatalf("price cells = %d, want %d\n%s", len(prices), len(priceWant), plain)
	}
	for i, cell := range prices {
		if !strings.HasSuffix(strings.TrimRight(cell, " "), priceWant[i]) {
			t.Fatalf("price cell %d = %q, want suffix %q\n%s", i, cell, priceWant[i], plain)
		}
		if !cellRightAligned(cell) {
			t.Fatalf("price cell %d is not flush right: %q\n%s", i, cell, plain)
		}
	}

	degen := renderPlain(t, book, true)
	degenValues := valueColumn(t, degen)
	if len(degenValues) != len(values) {
		t.Fatalf("degen value cells = %d, want %d\n%s", len(degenValues), len(values), degen)
	}
	for i, cell := range degenValues {
		if !cellRightAligned(cell) || !strings.HasSuffix(strings.TrimRight(cell, " "), want[i]) {
			t.Fatalf("degen cell %d = %q\n%s", i, cell, degen)
		}
	}
}

func renderPlain(t *testing.T, book *Book, degen bool) string {
	t.Helper()
	book.Degen = degen
	var buf bytes.Buffer
	u := ui.NewTerminalUIWithWriter(&buf, true)
	Render(u, book)
	return ansi.Strip(buf.String())
}

func valueColumn(t *testing.T, plain string) []string {
	t.Helper()
	return columnAt(t, plain, 4)
}

func columnAt(t *testing.T, plain string, index int) []string {
	t.Helper()
	var cells []string
	for _, line := range strings.Split(strings.TrimRight(plain, "\n"), "\n") {
		parts := strings.Split(line, "│")
		if len(parts) <= index {
			continue
		}
		cells = append(cells, parts[index])
	}
	return cells
}

func cellRightAligned(cell string) bool {
	if len(cell) < 2 || cell[0] != ' ' || cell[len(cell)-1] != ' ' {
		return false
	}
	inner := cell[1 : len(cell)-1]
	return inner == "" || inner[len(inner)-1] != ' '
}
