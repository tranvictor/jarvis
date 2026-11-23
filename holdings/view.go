package holdings

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/ui"
)

// Render prints the book. Live mode colours the value cell by the last
// tick; the 24h column follows the sign of the window change.
func Render(u ui.UI, b *Book) {
	now := time.Now()
	renderHeader(u, b)
	rows, omitted := visibleRows(b, now)
	if len(rows) == 0 {
		u.Info("No positions.")
	} else {
		headers := []string{"chain", "token", "amount", "value", "24h", "mark", "exit"}
		if b.Degen {
			headers = append(headers, "address", "raw", "spread", "age", "1h")
		}
		u.PrintTable(&ui.Table{Headers: headers, Rows: rows})
	}
	if omitted > 0 {
		u.Info("%s", u.Style(ui.StyledText{
			Text:     fmt.Sprintf("%d unpriced tokens omitted", omitted),
			Severity: ui.SeverityMuted,
		}))
	}
	lines := b.warningLines()
	capAt := 16
	if b.Degen {
		capAt = len(lines)
	}
	shown := lines
	extra := 0
	if len(lines) > capAt {
		shown = lines[:capAt]
		extra = len(lines) - capAt
	}
	for _, line := range shown {
		u.Warn("%s", line)
	}
	if extra > 0 {
		u.Warn("! %d more", extra)
	}
	if b.Live {
		u.Info("%s", u.Style(ui.StyledText{Text: liveStatus(b, now), Severity: ui.SeverityMuted}))
	}
}

func renderHeader(u ui.UI, b *Book) {
	name := headerName(b)
	total, totalSev := moneyText(b.Total, b.TotalTick)
	head := name + "    " + u.Style(ui.StyledText{Text: total + " USD", Severity: totalSev})
	if text, sev := pnlText(b.ChangeUSD, b.ChangePct); text != "" {
		head += "    " + u.Style(ui.StyledText{Text: text, Severity: sev}) + " 24h"
	}
	u.Info("%s", head)

	chains := "chains"
	if b.ChainCount == 1 {
		chains = "chain"
	}
	meta := fmt.Sprintf("%d positions · %d %s", b.Priced, b.ChainCount, chains)
	if b.Hidden > 0 {
		meta += fmt.Sprintf(" · %d under %s hidden", b.Hidden, formatUSD(b.MinUSD))
	}
	if b.Unpriced > 0 {
		meta += fmt.Sprintf(" · %d unpriced", b.Unpriced)
	}
	meta += " · spam and honeypots already removed"
	u.Info("%s", u.Style(ui.StyledText{Text: meta, Severity: ui.SeverityMuted}))

	amounts := "amounts on-chain"
	if len(b.KyberChains) > 0 {
		if b.Priced+b.Unpriced > 0 && len(b.KyberChains) > 0 {
			amounts = "amounts on-chain · Kyber amounts on " + strings.Join(b.KyberChains, ", ")
		}
	}
	if b.Priced+b.Unpriced > 0 && allKyber(b) {
		amounts = "amounts from Kyber"
	}
	u.Info("%s", u.Style(ui.StyledText{Text: amounts, Severity: ui.SeverityMuted}))
}

func allKyber(b *Book) bool {
	n := 0
	kyber := 0
	for _, p := range b.Positions {
		if p.Value != nil && *p.Value < b.MinUSD {
			continue
		}
		n++
		if p.AmountSource != amountChain {
			kyber++
		}
	}
	return n > 0 && kyber == n
}

func headerName(b *Book) string {
	desc := b.Name
	if strings.EqualFold(desc, "unknown") {
		desc = ""
	}
	return jarviscommon.NameFirst(jarviscommon.Address{Address: b.Address, Desc: desc}, b.Degen)
}

func visibleRows(b *Book, now time.Time) ([][]ui.TableCell, int) {
	var rows [][]ui.TableCell
	var unpriced []*Position
	for _, p := range b.Positions {
		if p.Value == nil {
			unpriced = append(unpriced, p)
			continue
		}
		if *p.Value < b.MinUSD {
			continue
		}
		rows = append(rows, rowFor(b, p, now))
	}
	limit := len(unpriced)
	omitted := 0
	if !b.Degen && limit > 8 {
		omitted = limit - 8
		limit = 8
	}
	for i := 0; i < limit; i++ {
		rows = append(rows, rowFor(b, unpriced[i], now))
	}
	return rows, omitted
}

func rowFor(b *Book, p *Position, now time.Time) []ui.TableCell {
	tick := 0
	if b.ValueTicks != nil {
		tick = b.ValueTicks[p.Key()]
	}
	value := ui.TCS("—", ui.SeverityMuted)
	if p.Value != nil {
		text, sev := moneyText(*p.Value, tick)
		value = ui.TCS(text, sev)
	}
	pct, pctSev := pctText(p.Change24h)
	mark := p.Mark
	if mark == "" {
		mark = "—"
	}
	exit := p.ExitTier
	exitSev := ui.SeverityInfo
	if exit == "" {
		exit = "—"
		exitSev = ui.SeverityMuted
	} else if exit == "thin" {
		exitSev = ui.SeverityWarn
	} else if exit == "none" {
		exitSev = ui.SeverityError
	}
	row := []ui.TableCell{
		ui.TC(p.ChainName),
		ui.TC(displaySymbol(p)),
		ui.TC(jarviscommon.CompactAmount(p.Amount)),
		value,
		ui.TCS(pct, pctSev),
		ui.TC(mark),
		ui.TCS(exit, exitSev),
	}
	if !b.Degen {
		return row
	}
	spread := "—"
	if bps, ok := p.spreadBps(); ok {
		spread = strconv.FormatFloat(bps, 'f', 0, 64) + " bps"
	}
	h1, h1Sev := pctText(p.Change1h)
	row = append(row,
		ui.TC(p.TokenAddress),
		ui.TC(p.RawAmount),
		ui.TC(spread),
		ui.TC(ageText(p.PriceAtMs, now)),
		ui.TCS(h1, h1Sev),
	)
	return row
}

func liveStatus(b *Book, now time.Time) string {
	parts := []string{"live"}
	if b.LiveNote != "" {
		parts = append(parts, b.LiveNote)
	}
	if !b.HeartbeatAt.IsZero() {
		parts = append(parts, "heartbeat "+ageText(b.HeartbeatAt.UnixMilli(), now))
	}
	if !b.BalancesAt.IsZero() {
		parts = append(parts, "balances "+ageText(b.BalancesAt.UnixMilli(), now))
	}
	if !b.StatsAt.IsZero() {
		parts = append(parts, "24h "+ageText(b.StatsAt.UnixMilli(), now))
	}
	return strings.Join(parts, " · ")
}
