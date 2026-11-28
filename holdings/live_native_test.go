package holdings

import (
	"testing"

	"github.com/tranvictor/jarvis/kyber"
)

// Ways a native onchain frame can fail to move the row:
//   - The subscribe name is not chain:address@onchainPrice, so this frame never arrives.
//   - s is parsed into the wrong chain or address, so find misses the row.
//   - The frame is dropped because the other side's timestamp is already newer,
//     even though this frame's sell is newer than the sell on the row.
//   - A newer sell already stored is replaced by this older one.
//   - The same timestamp and the same prices redraw the row.

func TestNativeFrameSubscribeNameAndMatch(t *testing.T) {
	v := 2000.0
	p := &Position{
		ChainID:      10,
		TokenAddress: "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",
		Amount:       "1",
		Value:        &v,
	}
	b := &Book{MinUSD: 1, Positions: []*Position{p}}
	streams := streamsFor(b)
	const want = "10:0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee@onchainPrice"
	found := false
	for _, s := range streams {
		if s == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("subscribe names %v, want %s", streams, want)
	}
	if b.find(10, "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee") != p {
		t.Fatal("s does not match the native row")
	}
}

func TestNativeSellLandsWhenBuyTimestampIsAhead(t *testing.T) {
	v := 1700.0
	p := &Position{
		ChainID:      10,
		TokenAddress: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Amount:       "1",
		BuyUSD:       "1700",
		BuyAtMs:      1795879666553 + 5000,
		OnchainE:     1795879666553 + 5000,
		Indicative:   "1700",
		Value:        &v,
	}
	b := &Book{Positions: []*Position{p}, MinUSD: 1}
	if !b.Apply(nativeOnchain()) {
		t.Fatal("frame dropped")
	}
	if p.SellUSD != "1645.159220703940274674" || p.Mark != "sell" || p.Price != p.SellUSD {
		t.Fatalf("sell did not become the visible price: %+v", p)
	}
	if p.BuyUSD != "1700" || p.BuyAtMs != 1795879666553+5000 {
		t.Fatalf("newer buy was overwritten: %+v", p)
	}
}

func TestOlderNativeSellDoesNotRegress(t *testing.T) {
	v := 1800.0
	p := &Position{
		ChainID:      10,
		TokenAddress: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Amount:       "1",
		SellUSD:      "1800",
		SellAtMs:     1795879666553 + 1000,
		BuyUSD:       "1801",
		BuyAtMs:      1795879666553 + 1000,
		OnchainE:     1795879666553 + 1000,
		Value:        &v,
	}
	b := &Book{Positions: []*Position{p}}
	if b.Apply(nativeOnchain()) {
		t.Fatal("older frame redrew the row")
	}
	if p.SellUSD != "1800" || p.BuyUSD != "1801" {
		t.Fatalf("prices regressed: %+v", p)
	}
}

func TestOmittedSellKeepsANewerSell(t *testing.T) {
	ev := nativeOnchain()
	ev.HasSell = false
	ev.Sell = ""
	ev.SellAt = 0
	v := 1800.0
	p := &Position{
		ChainID:      10,
		TokenAddress: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Amount:       "1",
		SellUSD:      "1800",
		SellAtMs:     ev.Time + 1000,
		Value:        &v,
	}
	b := &Book{Positions: []*Position{p}}
	b.Apply(ev)
	if p.SellUSD != "1800" || p.SellAtMs != ev.Time+1000 {
		t.Fatalf("sell cleared: %+v", p)
	}
}

func TestSameNativeFrameDoesNotRedraw(t *testing.T) {
	ev := nativeOnchain()
	v := 1645.0
	p := &Position{
		ChainID:      10,
		TokenAddress: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Amount:       "1",
		SellUSD:      ev.Sell,
		SellAtMs:     ev.SellAt,
		BuyUSD:       ev.Buy,
		BuyAtMs:      ev.BuyAt,
		OnchainE:     ev.Time,
		Value:        &v,
	}
	b := &Book{Positions: []*Position{p}}
	if b.Apply(ev) {
		t.Fatal("duplicate frame redrew the row")
	}
}

func nativeOnchain() kyber.MarketEvent {
	return kyber.MarketEvent{
		Kind:    "onchainPrice",
		Time:    1795879666553,
		ChainID: 10,
		Address: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		HasBuy:  true,
		Buy:     "1645.846133594868421033",
		BuyAt:   1795879666553,
		HasSell: true,
		Sell:    "1645.159220703940274674",
		SellAt:  1795879666553,
	}
}
