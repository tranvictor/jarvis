package holdings

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/tranvictor/jarvis/ui"
)

func TestHoneypotIsHiddenFromTheBook(t *testing.T) {
	eth := 100.0
	scam := 88637957.66
	tax := 50.0
	unchecked := 10.0
	yes := true
	book := &Book{
		Address: "0xa6c8000000000000000000000000000000005e6e",
		Name:    "tranvictor.eth",
		MinUSD:  1,
		Positions: []*Position{
			{ChainID: 56, ChainName: "bsc", TokenAddress: "0x1111111111111111111111111111111111111111", Symbol: "567Ucom", Amount: "10.567", Value: &scam, Mark: "sell", ExitTier: "none", IsHoneypot: &yes, RiskLevel: "critical", RiskReasons: []string{"HONEYPOT", "TAX_UNMEASURED"}},
			{ChainID: 56, ChainName: "bsc", TokenAddress: "0x2222222222222222222222222222222222222222", Symbol: "BNB", Amount: "1", Value: &eth, Mark: "sell"},
			{ChainID: 1, ChainName: "ethereum", TokenAddress: "0x3333333333333333333333333333333333333333", Symbol: "TAX", Amount: "1", Value: &tax, Mark: "sell", RiskLevel: "critical", RiskReasons: []string{"SELL_TAX_EXTREME"}},
			{ChainID: 1, ChainName: "ethereum", TokenAddress: "0x4444444444444444444444444444444444444444", Symbol: "MAYBE", Amount: "1", Value: &unchecked, Mark: "sell", RiskReasons: []string{"HONEYPOT_UNCHECKED"}},
			{ChainID: 137, ChainName: "polygon", TokenAddress: "0x5555555555555555555555555555555555555555", Symbol: "GHOST", Amount: "1", RiskLevel: "critical", RiskReasons: []string{"HONEYPOT"}},
		},
	}
	book.recomputeTotals()
	if book.Honeypots != 2 {
		t.Fatalf("honeypots = %d, want 2", book.Honeypots)
	}
	if book.Total != eth+tax+unchecked {
		t.Fatalf("total = %v, want %v", book.Total, eth+tax+unchecked)
	}
	if book.Priced != 3 {
		t.Fatalf("priced = %d, want 3", book.Priced)
	}
	if book.ChainCount != 2 {
		t.Fatalf("chains = %d, want 2", book.ChainCount)
	}

	var buf bytes.Buffer
	Render(ui.NewTerminalUIWithWriter(&buf, false), book)
	plain := ansi.Strip(buf.String())
	for _, banned := range []string{"567Ucom", "GHOST", "88,637,957.66"} {
		if strings.Contains(plain, banned) {
			t.Fatalf("book still shows %q\n%s", banned, plain)
		}
	}
	for _, want := range []string{"BNB", "TAX", "MAYBE", "2 honeypots hidden", "160.00"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("book missing %q\n%s", want, plain)
		}
	}
	if !strings.Contains(plain, "ethereum TAX") {
		t.Fatalf("a non-honeypot critical token should still warn\n%s", plain)
	}

	snap := book.Snapshot()
	if snap.HiddenHoneypots != 2 {
		t.Fatalf("json honeypots = %d", snap.HiddenHoneypots)
	}
	if len(snap.Positions) != 3 {
		t.Fatalf("json positions = %d, want 3", len(snap.Positions))
	}
	for _, p := range snap.Positions {
		if p.Symbol == "567Ucom" || p.Symbol == "GHOST" {
			t.Fatalf("json still has %s", p.Symbol)
		}
	}
}
