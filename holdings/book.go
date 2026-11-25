package holdings

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/ui"
)

const (
	glyphUp   = "▲"
	glyphDown = "▼"
)

// Position is one holding after marks, chain amounts and exit tier.
type Position struct {
	ChainID      int
	ChainName    string
	TokenAddress string
	Symbol       string
	Decimals     int
	Amount       string
	RawAmount    string
	AmountSource string
	Native       bool
	QuoteSymbol  string

	Value     *float64
	Mark      string
	Price     string
	PriceAtMs int64

	SellUSD  string
	SellAtMs int64
	BuyUSD   string
	BuyAtMs  int64
	OnchainE int64

	SettlementUSD   string
	SettlementAtMs  int64
	SettlementEpoch int64
	SettlementBlock int64
	HasSettlement   bool

	Change24h  *float64
	Change1h   *float64
	Indicative string

	RiskLevel   string
	RiskReasons []string
	SellTaxBps  *int
	BuyTaxBps   *int
	IsFOT       *bool
	IsRebasing  *bool
	IsHoneypot  *bool
	ExitTier    string
}

// Key identifies a holding inside one book.
func (p *Position) Key() string {
	return strconv.Itoa(p.ChainID) + ":" + strings.ToLower(p.TokenAddress)
}

func (p *Position) revalue() {
	switch {
	case p.SellUSD != "":
		p.Mark = "sell"
		p.Price = p.SellUSD
		p.PriceAtMs = p.SellAtMs
	case p.SettlementUSD != "":
		p.Mark = "settle"
		p.Price = p.SettlementUSD
		p.PriceAtMs = p.SettlementAtMs
	case p.Indicative != "":
		p.Mark = "indicative"
		p.Price = p.Indicative
		p.PriceAtMs = 0
	default:
		p.Mark = ""
		p.Price = ""
		p.PriceAtMs = 0
		p.Value = nil
		return
	}
	v, ok := mulDec(p.Amount, p.Price)
	if !ok {
		p.Value = nil
		return
	}
	p.Value = &v
}

func (p *Position) spreadBps() (float64, bool) {
	if p.BuyUSD == "" || p.SellUSD == "" {
		return 0, false
	}
	buy, ok1 := new(big.Rat).SetString(p.BuyUSD)
	sell, ok2 := new(big.Rat).SetString(p.SellUSD)
	if !ok1 || !ok2 || sell.Sign() == 0 {
		return 0, false
	}
	diff := new(big.Rat).Sub(buy, sell)
	if diff.Sign() < 0 {
		diff.Neg(diff)
	}
	diff.Quo(diff, sell)
	diff.Mul(diff, big.NewRat(10000, 1))
	f, _ := diff.Float64()
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// Book is the portfolio the screen and the JSON file both render.
type Book struct {
	Address   string
	Name      string
	MinUSD    float64
	Positions []*Position

	Total       float64
	ChangeUSD   *float64
	ChangePct   *float64
	Hidden      int
	Honeypots   int
	Unpriced    int
	Priced      int
	ChainCount  int
	KyberChains []string

	MarketNote   string
	SecurityNote string
	Pipeline     map[int]pipeline
	Degraded     map[int]map[string]bool

	FetchedAt   time.Time
	BalancesAt  time.Time
	StatsAt     time.Time
	HeartbeatAt time.Time

	Degen      bool
	Live       bool
	LiveNote   string
	TotalTick  int
	ValueTicks map[string]int
}

type pipeline struct {
	name       string
	onchain    bool
	settlement bool
	stats      bool
}

func (b *Book) find(chainID int, address string) *Position {
	address = strings.ToLower(address)
	for _, p := range b.Positions {
		if p.ChainID == chainID && strings.ToLower(p.TokenAddress) == address {
			return p
		}
	}
	return nil
}

func (b *Book) recomputeTotals() {
	var total, change, base float64
	hasChange := false
	hidden, unpriced, priced := 0, 0, 0
	chains := map[int]bool{}
	kyber := map[string]bool{}
	honeypots := 0
	for _, p := range b.Positions {
		if p.honeypot() {
			honeypots++
			continue
		}
		if p.Value == nil {
			unpriced++
			chains[p.ChainID] = true
			if p.AmountSource != amountChain {
				kyber[p.ChainName] = true
			}
			continue
		}
		if *p.Value < b.MinUSD {
			hidden++
			continue
		}
		priced++
		chains[p.ChainID] = true
		total += *p.Value
		if p.AmountSource != amountChain {
			kyber[p.ChainName] = true
		}
		if p.Change24h != nil {
			hasChange = true
			change += *p.Value * (*p.Change24h / 100)
			base += *p.Value
		}
	}
	b.Total = total
	b.Hidden = hidden
	b.Honeypots = honeypots
	b.Unpriced = unpriced
	b.Priced = priced
	b.ChainCount = len(chains)
	if hasChange && base > 0 {
		c := change
		pct := change / base * 100
		b.ChangeUSD = &c
		b.ChangePct = &pct
	} else {
		b.ChangeUSD = nil
		b.ChangePct = nil
	}
	b.KyberChains = b.KyberChains[:0]
	for name := range kyber {
		b.KyberChains = append(b.KyberChains, name)
	}
	sortStrings(b.KyberChains)
}

func (b *Book) sortPositions() {
	chainTotal := map[int]float64{}
	for _, p := range b.Positions {
		if p.honeypot() {
			continue
		}
		if p.Value != nil && *p.Value >= b.MinUSD {
			chainTotal[p.ChainID] += *p.Value
		}
	}
	sortPositions(b.Positions, chainTotal)
}

func (b *Book) degraded(chainID int, kind string) bool {
	if b.Degraded == nil {
		return false
	}
	return b.Degraded[chainID][kind]
}

// warningLines is the ! block under the table.
func (b *Book) warningLines() []string {
	var lines []string
	if b.MarketNote != "" {
		lines = append(lines, "! "+b.MarketNote)
	}
	if b.SecurityNote != "" {
		lines = append(lines, "! "+b.SecurityNote)
	}
	var ids []int
	for id, p := range b.Pipeline {
		if p.onchain || p.settlement || p.stats {
			ids = append(ids, id)
		}
	}
	sortInts(ids)
	for _, id := range ids {
		p := b.Pipeline[id]
		var what []string
		if p.onchain {
			what = append(what, "onchain prices")
		}
		if p.settlement {
			what = append(what, "settlement prices")
		}
		if p.stats {
			what = append(what, "24h stats")
		}
		name := p.name
		if name == "" {
			name = "chain " + strconv.Itoa(id)
		}
		lines = append(lines, "! "+name+"  "+strings.Join(what, ", ")+" degraded")
	}
	for _, p := range b.Positions {
		if p.honeypot() {
			continue
		}
		if p.Value != nil && *p.Value < b.MinUSD {
			continue
		}
		lines = append(lines, positionWarnings(p)...)
	}
	return lines
}

func positionWarnings(p *Position) []string {
	var lines []string
	label := p.ChainName + " " + displaySymbol(p)
	if risk := riskText(p); risk != "" {
		lines = append(lines, "! "+label+"  "+risk)
	}
	if p.IsRebasing != nil && *p.IsRebasing {
		lines = append(lines, "! "+label+"  rebasing · the balance can move with no transfer")
	}
	if bps, ok := p.spreadBps(); ok && bps > 200 {
		lines = append(lines, "! "+label+"  buy/sell spread "+strconv.FormatFloat(bps, 'f', 0, 64)+" bps")
	}
	if p.ExitTier == "thin" || p.ExitTier == "none" {
		into := "thin exit"
		if p.ExitTier == "none" {
			into = "no exit"
		}
		if p.QuoteSymbol != "" {
			into += " into " + p.QuoteSymbol
		}
		lines = append(lines, "! "+label+"  "+into)
	}
	return lines
}

// honeypot is the catalog verdict that the token can be bought and not sold.
// HONEYPOT_UNCHECKED is a missing check, not a verdict.
func (p *Position) honeypot() bool {
	if p == nil {
		return false
	}
	if p.IsHoneypot != nil && *p.IsHoneypot {
		return true
	}
	for _, r := range p.RiskReasons {
		if r == "HONEYPOT" {
			return true
		}
	}
	return false
}

func riskText(p *Position) string {
	level := strings.ToLower(p.RiskLevel)
	honeypot := p.IsHoneypot != nil && *p.IsHoneypot
	fot := p.IsFOT != nil && *p.IsFOT
	sell := p.SellTaxBps != nil && *p.SellTaxBps >= 100
	buy := p.BuyTaxBps != nil && *p.BuyTaxBps >= 100
	hot := level == "high" || level == "critical" || honeypot || fot || sell || buy
	if !hot {
		return ""
	}
	var bits []string
	if level != "" {
		bits = append(bits, "risk "+level)
	} else if honeypot {
		bits = append(bits, "risk critical")
	}
	for _, r := range p.RiskReasons {
		if r == "" {
			continue
		}
		bits = append(bits, r)
	}
	if p.SellTaxBps != nil && *p.SellTaxBps > 0 {
		bits = append(bits, "sell tax "+strconv.Itoa(*p.SellTaxBps)+" bps")
	}
	if p.BuyTaxBps != nil && *p.BuyTaxBps > 0 {
		bits = append(bits, "buy tax "+strconv.Itoa(*p.BuyTaxBps)+" bps")
	}
	if fot {
		bits = append(bits, "fee-on-transfer")
	}
	return strings.Join(bits, " · ")
}

func displaySymbol(p *Position) string {
	if p.Symbol != "" {
		return p.Symbol
	}
	return jarviscommon.ShortAddress(p.TokenAddress)
}

func pctText(p *float64) (string, ui.Severity) {
	if p == nil {
		return "—", ui.SeverityMuted
	}
	if math.Abs(*p) < 0.05 {
		return "0.0%", ui.SeverityInfo
	}
	if *p > 0 {
		return glyphUp + " " + strconv.FormatFloat(*p, 'f', 1, 64) + "%", ui.SeveritySuccess
	}
	return glyphDown + " " + strconv.FormatFloat(-*p, 'f', 1, 64) + "%", ui.SeverityError
}

func moneyText(v float64, tick int, reserve bool) (string, ui.Severity) {
	text := formatUSD(v)
	switch tick {
	case 1:
		return glyphUp + " " + text, ui.SeveritySuccess
	case -1:
		return glyphDown + " " + text, ui.SeverityError
	default:
		if reserve {
			return "  " + text, ui.SeverityInfo
		}
		return text, ui.SeverityInfo
	}
}

func pnlText(usd, pct *float64) (string, ui.Severity) {
	if usd == nil || pct == nil {
		return "", ui.SeverityInfo
	}
	if math.Abs(*pct) < 0.05 && math.Abs(*usd) < 0.005 {
		return formatUSD(0) + " (0.0%)", ui.SeverityInfo
	}
	glyph := glyphUp
	sev := ui.SeveritySuccess
	absUSD := *usd
	if *usd < 0 || *pct < 0 {
		glyph = glyphDown
		sev = ui.SeverityError
		absUSD = math.Abs(*usd)
	}
	pctAbs := math.Abs(*pct)
	digits := 1
	if pctAbs < 0.05 && math.Abs(*usd) >= 0.005 {
		digits = 2
	}
	text := glyph + " " + formatUSD(absUSD) + " (" + glyph + " " + strconv.FormatFloat(pctAbs, 'f', digits, 64) + "%)"
	return text, sev
}

func formatUSD(v float64) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return sign + jarviscommon.GroupDigits(strconv.FormatFloat(v, 'f', 2, 64))
}

func ageText(ms int64, now time.Time) string {
	if ms <= 0 {
		return "—"
	}
	d := now.Sub(time.UnixMilli(ms))
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	}
}

func mulDec(a, b string) (float64, bool) {
	ar, ok1 := new(big.Rat).SetString(strings.TrimSpace(a))
	br, ok2 := new(big.Rat).SetString(strings.TrimSpace(b))
	if !ok1 || !ok2 {
		return 0, false
	}
	ar.Mul(ar, br)
	f, _ := ar.Float64()
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func humanAmount(raw *big.Int, decimals int) string {
	if raw == nil {
		return "0"
	}
	r := new(big.Rat).SetInt(raw)
	if decimals > 0 {
		den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
		r.Quo(r, new(big.Rat).SetInt(den))
	}
	s := r.FloatString(18)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

func zeroAmount(s string) bool {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return true
	}
	return r.Sign() == 0
}
