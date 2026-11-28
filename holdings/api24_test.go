package holdings

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/kyber"
)

// Ways the 24h percent can fail:
//
//   - stats["24h"] missing, or priceChangePercent null: no arrow on a
//     native row or any other row, and a previous percent is cleared.
//   - A move under 0.05% is stored and stays flat. The decimals stay aligned.
//   - The same field sets the arrow on a native 0xeeee row and an ERC-20 row.

func TestMarketPercentSetsArrowsOnEveryRow(t *testing.T) {
	native := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	usdc := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	book := pricedBook(native, usdc, "1645.159", "1.000")
	applyMarket(book.Positions[0], marketPercent(1.2))
	applyMarket(book.Positions[1], marketPercent(-4.2))
	book.recomputeTotals()

	if book.Positions[0].Change24h == nil || *book.Positions[0].Change24h != 1.2 {
		t.Fatalf("native change = %v", book.Positions[0].Change24h)
	}
	if book.Positions[1].Change24h == nil || *book.Positions[1].Change24h != -4.2 {
		t.Fatalf("usdc change = %v", book.Positions[1].Change24h)
	}

	plain := renderPlain(t, book, false)
	prices := columnAt(t, plain, 5)
	if len(prices) < 3 || !strings.Contains(prices[1], "▲") || !strings.Contains(prices[2], "▼") {
		t.Fatalf("price cells:\n%s", plain)
	}
	assertPriceDecimalsAlign(t, prices[1:])
}

func TestNullPercentClearsArrow(t *testing.T) {
	native := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	usdc := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	book := pricedBook(native, usdc, "1645.159", "1.000")
	kept := 9.5
	book.Positions[0].Change24h = &kept
	book.Positions[1].Change24h = &kept
	applyMarket(book.Positions[0], marketPercentJSON(`{"stats":{"24h":{"priceChangePercent":null},"1h":{"priceChangePercent":1}}}`))
	applyMarket(book.Positions[1], marketPercentJSON(`{"stats":null}`))

	if book.Positions[0].Change24h != nil || book.Positions[1].Change24h != nil {
		t.Fatalf("null percent left a change: native %v usdc %v", book.Positions[0].Change24h, book.Positions[1].Change24h)
	}
	if book.Positions[0].Change1h == nil || *book.Positions[0].Change1h != 1 {
		t.Fatalf("1h = %v, want 1", book.Positions[0].Change1h)
	}
	plain := renderPlain(t, book, false)
	prices := columnAt(t, plain, 5)
	for _, cell := range prices[1:] {
		if strings.Contains(cell, "▲") || strings.Contains(cell, "▼") {
			t.Fatalf("null percent drew an arrow:\n%s", plain)
		}
	}
	assertPriceDecimalsAlign(t, prices[1:])
}

func TestAPIPercentUnderCutoffHasNoArrow(t *testing.T) {
	native := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	usdc := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	book := pricedBook(native, usdc, "2940.500", "1.0001")
	applyMarket(book.Positions[0], marketPercent(0.04))
	applyMarket(book.Positions[1], marketPercent(-0.04))
	book.recomputeTotals()

	for _, p := range book.Positions {
		if p.Change24h == nil || windowTick(p.Change24h) != 0 {
			t.Fatalf("%s tick from %v", p.Symbol, p.Change24h)
		}
	}
	plain := renderPlain(t, book, false)
	prices := columnAt(t, plain, 5)
	for _, cell := range prices[1:] {
		if strings.Contains(cell, "▲") || strings.Contains(cell, "▼") {
			t.Fatalf("flat percent drew an arrow:\n%s", plain)
		}
	}
	assertPriceDecimalsAlign(t, prices[1:])
}

func marketPercent(v float64) kyber.TokenMarket {
	return marketPercentJSON(`{"stats":{"24h":{"priceChangePercent":` + jsonNumber(v) + `}}}`)
}

func marketPercentJSON(body string) kyber.TokenMarket {
	var tm kyber.TokenMarket
	if err := json.Unmarshal([]byte(body), &tm); err != nil {
		panic(err)
	}
	return tm
}

func jsonNumber(v float64) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func pricedBook(native, usdc, nativePx, usdcPx string) *Book {
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
