package holdings

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tranvictor/jarvis/kyber"
)

// Ways the 24h mark can fail:
//
//   - Nothing recorded 24h ago: no arrow on a native row or any other row.
//   - The only older price is 23h old: still no arrow.
//   - A zero or unparseable mark: no arrow, and the book does not panic.
//   - A move under 0.05% is flat. The decimals stay in one column.
//   - stats["24h"].priceChangePercent is ignored.
//   - A new book reads the same file and uses the stored mark.

func TestMark24hSetsArrowsOnEveryRow(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000).UTC()
	native := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	usdc := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	book := markedBook(native, usdc, "100", "100")
	book.notePrices(now.Add(-priceMarkWindow))

	book.Positions[0].SellUSD = "110"
	book.Positions[1].SellUSD = "90"
	book.Positions[0].revalue()
	book.Positions[1].revalue()
	book.notePrices(now)
	book.recomputeTotals()

	if book.Positions[0].Change24h == nil || *book.Positions[0].Change24h < 9 {
		t.Fatalf("native change = %v, want about +10", book.Positions[0].Change24h)
	}
	if book.Positions[1].Change24h == nil || *book.Positions[1].Change24h > -9 {
		t.Fatalf("usdc change = %v, want about -10", book.Positions[1].Change24h)
	}
	if book.ChangePct == nil {
		t.Fatal("header change was not set from the row marks")
	}

	plain := renderPlain(t, book, false)
	prices := columnAt(t, plain, 5)
	if len(prices) < 3 {
		t.Fatalf("price cells:\n%s", plain)
	}
	if !strings.Contains(prices[1], "▲") || !strings.Contains(prices[1], "110.000") {
		t.Fatalf("native price = %q\n%s", prices[1], plain)
	}
	if !strings.Contains(prices[2], "▼") || !strings.Contains(prices[2], "90.000") {
		t.Fatalf("usdc price = %q\n%s", prices[2], plain)
	}
	assertPriceDecimalsAlign(t, prices[1:3])
}

func TestMarkYoungerThan24hStaysFlat(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000).UTC()
	native := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	usdc := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	book := markedBook(native, usdc, "100", "2")
	book.notePrices(now.Add(-23 * time.Hour))
	book.Positions[0].SellUSD = "110"
	book.Positions[1].SellUSD = "3"
	book.Positions[0].revalue()
	book.Positions[1].revalue()
	book.notePrices(now)

	if book.Positions[0].Change24h != nil || book.Positions[1].Change24h != nil {
		t.Fatalf("young marks set change: native %v usdc %v", book.Positions[0].Change24h, book.Positions[1].Change24h)
	}
	plain := renderPlain(t, book, false)
	prices := columnAt(t, plain, 5)
	for i, cell := range prices[1:] {
		if strings.Contains(cell, "▲") || strings.Contains(cell, "▼") {
			t.Fatalf("price cell %d has an arrow: %q\n%s", i, cell, plain)
		}
	}
}

func TestMarkUnderCutoffHasNoArrow(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000).UTC()
	native := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	usdc := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	book := markedBook(native, usdc, "100", "100")
	book.notePrices(now.Add(-priceMarkWindow))
	book.Positions[0].SellUSD = "100.04"
	book.Positions[1].SellUSD = "100.04"
	book.Positions[0].revalue()
	book.Positions[1].revalue()
	book.notePrices(now)

	for _, p := range book.Positions {
		if p.Change24h == nil || math.Abs(*p.Change24h) >= 0.05 {
			t.Fatalf("%s change = %v, want a stored move under 0.05", p.Symbol, p.Change24h)
		}
		if windowTick(p.Change24h) != 0 {
			t.Fatalf("%s tick = %d, want flat", p.Symbol, windowTick(p.Change24h))
		}
	}
	plain := renderPlain(t, book, false)
	prices := columnAt(t, plain, 5)
	assertPriceDecimalsAlign(t, prices[1:])
	for i, cell := range prices[1:] {
		if strings.Contains(strings.TrimSpace(cell), "▲") || strings.Contains(strings.TrimSpace(cell), "▼") {
			t.Fatalf("flat price cell %d = %q\n%s", i, cell, plain)
		}
	}
}

func TestZeroMarkDoesNotPanic(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000).UTC()
	book := markedBook("0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "0", "nope")
	book.notePrices(now.Add(-priceMarkWindow))
	book.Positions[0].SellUSD = "5"
	book.Positions[1].SellUSD = "5"
	book.Positions[0].revalue()
	book.Positions[1].revalue()
	book.notePrices(now)
	if book.Positions[0].Change24h != nil || book.Positions[1].Change24h != nil {
		t.Fatalf("bad marks produced a change: %v %v", book.Positions[0].Change24h, book.Positions[1].Change24h)
	}
}

func TestAPIPercentIsNotTheArrow(t *testing.T) {
	var tm kyber.TokenMarket
	raw := []byte(`{"stats":{"24h":{"priceChangePercent":9.5},"1h":{"priceChangePercent":1}}}`)
	if err := json.Unmarshal(raw, &tm); err != nil {
		t.Fatal(err)
	}
	p := &Position{
		ChainID: 10, TokenAddress: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Symbol: "ETH", Amount: "1", SellUSD: "1645.159",
	}
	p.revalue()
	applyMarket(p, tm)
	if p.Change24h != nil {
		t.Fatalf("API 24h percent was stored: %v", *p.Change24h)
	}
	if p.Change1h == nil || *p.Change1h != 1 {
		t.Fatalf("1h = %v, want 1", p.Change1h)
	}
	book := &Book{Positions: []*Position{p}}
	book.notePrices(time.UnixMilli(1_800_000_000_000))
	if p.Change24h != nil {
		t.Fatalf("a fresh mark set a change: %v", *p.Change24h)
	}
}

func TestMarkSurvivesReload(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000).UTC()
	dir := t.TempDir()
	path := filepath.Join(dir, "price-24h.json")
	native := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	book := markedBook(native, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "1645", "1")
	book.marks = newPriceMarks(path)
	book.notePrices(now.Add(-priceMarkWindow))
	book.Positions[0].SellUSD = "1700"
	book.Positions[0].revalue()
	book.notePrices(now)

	next := markedBook(native, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "1700", "1")
	next.marks = loadPriceMarks(path)
	next.notePrices(now)
	if next.Positions[0].Change24h == nil || *next.Positions[0].Change24h < 3 {
		t.Fatalf("reloaded native change = %v", next.Positions[0].Change24h)
	}

	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	broken := markedBook(native, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "1700", "1")
	broken.marks = loadPriceMarks(path)
	broken.notePrices(now)
	if broken.Positions[0].Change24h != nil {
		t.Fatalf("corrupt file set a change: %v", *broken.Positions[0].Change24h)
	}
}

func TestSocketUpdateUses24hMark(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000).UTC()
	p := &Position{
		ChainID: 10, ChainName: "Optimism", Symbol: "ETH", Amount: "1",
		TokenAddress: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		SellUSD:      "1645.159220703940274674", SellAtMs: now.Add(-priceMarkWindow).UnixMilli(),
	}
	p.revalue()
	b := &Book{
		MinUSD: 1, Positions: []*Position{p},
		now: func() time.Time { return now.Add(-priceMarkWindow) },
	}
	b.notePrices(b.clock())

	b.now = func() time.Time { return now }
	ev := kyber.MarketEvent{
		Kind: "onchainPrice", Time: now.UnixMilli(), ChainID: 10,
		Address: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		HasBuy:  true, Buy: "1700", BuyAt: now.UnixMilli(),
		HasSell: true, Sell: "1800", SellAt: now.UnixMilli(),
	}
	if !b.Apply(ev) {
		t.Fatal("frame dropped")
	}
	if p.SellUSD != "1800" {
		t.Fatalf("sell = %s", p.SellUSD)
	}
	if p.Change24h == nil || *p.Change24h < 9 {
		t.Fatalf("socket change = %v, want about +9.4", p.Change24h)
	}
	plain := renderPlain(t, b, false)
	prices := columnAt(t, plain, 5)
	if len(prices) < 2 || !strings.Contains(prices[1], "▲") {
		t.Fatalf("native price cell:\n%s", plain)
	}
}

func markedBook(native, usdc, nativePx, usdcPx string) *Book {
	eth := 1.0
	usd := 1.0
	a := &Position{
		ChainID: 10, ChainName: "Optimism", Symbol: "ETH", Amount: "1", Value: &eth,
		TokenAddress: native, SellUSD: nativePx,
	}
	b := &Position{
		ChainID: 10, ChainName: "Optimism", Symbol: "USDC", Amount: "1", Value: &usd,
		TokenAddress: usdc, SellUSD: usdcPx,
	}
	a.revalue()
	b.revalue()
	return &Book{
		Address: "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045", Name: "vitalik.eth",
		MinUSD: 1, Positions: []*Position{a, b},
	}
}
