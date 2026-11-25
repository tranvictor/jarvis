package holdings

import (
	"strings"
	"testing"
)

func TestDegenPrintsLiquidityPayload(t *testing.T) {
	usdt := 5000.0
	knc := 1200.0
	eth := 2000.0
	book := &Book{
		Address: "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
		MinUSD:  1,
		Positions: []*Position{
			{ChainID: 1, ChainName: "Ethereum", TokenAddress: "0xdac17f958d2ee523a2206206994597c13d831ec7", Symbol: "USDT", Amount: "5000", Value: &usdt, Mark: "sell", ExitTier: "none", ExitQuoted: true, ExitScore: 0.5, ExitVenues: 3},
			{ChainID: 1, ChainName: "Ethereum", TokenAddress: "0xdefa4e8a7bcba345f687a2f1456f5edd9ce97202", Symbol: "KNC", Amount: "100", Value: &knc, Mark: "sell", ExitTier: "thin", ExitQuoted: true, ExitScore: 12, ExitVenues: 1},
			{ChainID: 1, ChainName: "Ethereum", TokenAddress: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Symbol: "ETH", Amount: "1", Value: &eth, Native: true, Mark: "sell"},
		},
	}

	off := renderPlain(t, book, false)
	if strings.Contains(off, "venueCount") || strings.Contains(off, "not quoted") {
		t.Fatalf("payload printed without -x:\n%s", off)
	}
	if !strings.Contains(off, "none") || !strings.Contains(off, "thin") {
		t.Fatalf("exit cells missing:\n%s", off)
	}

	on := renderPlain(t, book, true)
	for _, want := range []string{
		`Ethereum USDT  {"tier":"none","score":0.5,"venueCount":3}`,
		`Ethereum KNC  {"tier":"thin","score":12,"venueCount":1}`,
		`Ethereum ETH  not quoted`,
	} {
		if !strings.Contains(on, want) {
			t.Fatalf("missing %q\n%s", want, on)
		}
	}
	if !strings.Contains(on, "none") {
		t.Fatalf("exit cell changed:\n%s", on)
	}
}
