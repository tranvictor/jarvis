package holdings

import (
	"time"

	"github.com/tranvictor/jarvis/kyber"
)

// Apply folds one socket event into the book. False means the frame
// changed nothing the screen needs to redraw.
func (b *Book) Apply(ev kyber.MarketEvent) bool {
	switch ev.Kind {
	case "heartbeat":
		if ev.Time > 0 {
			b.HeartbeatAt = time.UnixMilli(ev.Time)
		} else {
			b.HeartbeatAt = time.Now()
		}
		return false
	case "status":
		if b.Degraded == nil {
			b.Degraded = map[int]map[string]bool{}
		}
		if b.Degraded[ev.ChainID] == nil {
			b.Degraded[ev.ChainID] = map[string]bool{}
		}
		if b.Degraded[ev.ChainID][ev.StatusKind] == ev.Degraded {
			return false
		}
		b.Degraded[ev.ChainID][ev.StatusKind] = ev.Degraded
		return true
	case "onchainPrice":
		if b.degraded(ev.ChainID, "onchainPrice") {
			return false
		}
		p := b.find(ev.ChainID, ev.Address)
		if p == nil {
			return false
		}
		// Each side lands on its own time. A buy timestamp that is already
		// ahead must not throw away a newer sell: the row shows the sell,
		// then settlement, then the catalog price, and never the buy.
		changed := false
		if ev.HasBuy {
			if sideNewer(ev.BuyAt, p.BuyAtMs, ev.Buy, p.BuyUSD) {
				p.BuyUSD = ev.Buy
				p.BuyAtMs = ev.BuyAt
				changed = true
			}
		} else if sideGone(ev.Time, p.BuyAtMs, p.BuyUSD) {
			p.BuyUSD = ""
			p.BuyAtMs = 0
			changed = true
		}
		if ev.HasSell {
			if sideNewer(ev.SellAt, p.SellAtMs, ev.Sell, p.SellUSD) {
				p.SellUSD = ev.Sell
				p.SellAtMs = ev.SellAt
				changed = true
			}
		} else if sideGone(ev.Time, p.SellAtMs, p.SellUSD) {
			p.SellUSD = ""
			p.SellAtMs = 0
			changed = true
		}
		if !changed {
			return false
		}
		if p.BuyAtMs > p.OnchainE {
			p.OnchainE = p.BuyAtMs
		}
		if p.SellAtMs > p.OnchainE {
			p.OnchainE = p.SellAtMs
		}
		p.revalue()
		b.recomputeTotals()
		return true
	case "settlementPrice":
		if b.degraded(ev.ChainID, "settlementPrice") {
			return false
		}
		p := b.find(ev.ChainID, ev.Address)
		if p == nil || ev.Settlement == "" {
			return false
		}
		if p.HasSettlement {
			if ev.Epoch < p.SettlementEpoch {
				return false
			}
			if ev.Epoch == p.SettlementEpoch && ev.Block <= p.SettlementBlock {
				return false
			}
		}
		p.SettlementUSD = ev.Settlement
		p.SettlementAtMs = ev.Time
		p.SettlementEpoch = ev.Epoch
		p.SettlementBlock = ev.Block
		p.HasSettlement = true
		p.revalue()
		b.recomputeTotals()
		return true
	default:
		return false
	}
}

// sideNewer reports a socket side that should replace the one stored.
// Equal time with a different price counts. A missing time does not
// replace a side that has one.
func sideNewer(at, haveAt int64, price, have string) bool {
	if at > haveAt {
		return true
	}
	return at == haveAt && price != have
}

// sideGone reports an omitted side the frame is new enough to clear.
func sideGone(frameAt, haveAt int64, have string) bool {
	return have != "" && frameAt >= haveAt
}

func streamsFor(b *Book) []string {
	type item struct {
		p *Position
		v float64
	}
	var items []item
	for _, p := range b.Positions {
		if p.honeypot() || p.Value == nil || *p.Value < b.MinUSD {
			continue
		}
		items = append(items, item{p, *p.Value})
	}
	for i := 1; i < len(items); i++ {
		j := i
		for j > 0 && items[j].v > items[j-1].v {
			items[j], items[j-1] = items[j-1], items[j]
			j--
		}
	}
	var streams []string
	for _, it := range items {
		base := it.p.Key()
		if len(streams)+2 <= 200 {
			streams = append(streams, base+"@onchainPrice", base+"@settlementPrice")
			continue
		}
		if len(streams)+1 <= 200 {
			streams = append(streams, base+"@onchainPrice")
		}
		break
	}
	return streams
}

func sameStreams(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
