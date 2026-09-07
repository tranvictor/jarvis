package util

import (
	"fmt"
	"regexp"
	"strings"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
)

// collapseAbove is the element count past which arrays are summarised as
// "[n items]" in LayoutInfo. Small arrays (a swap path, a pair of recipients)
// are worth reading inline; long ones are noise until asked for with -x.
const collapseAbove = 4

// longHexLen is the text length past which a bare hex blob is shortened in
// compact layouts. A 32-byte word is 66 characters, so anything longer is
// calldata-like rather than a hash or address.
const longHexLen = 66

// addressWithDesc matches the "0x… (Desc)" form produced by PlainAddress so
// compact layouts can re-shape it without the view-model carrying a second,
// display-only representation of every address. The boundary groups keep a
// 40-digit run inside a longer hash or calldata blob from being mistaken for
// an address.
var addressWithDesc = regexp.MustCompile(`(^|[^0-9a-fA-Fx])(0x[0-9a-fA-F]{40})( \(([^()]*)\))?([^0-9a-fA-F]|$)`)

var descDecimalSuffix = regexp.MustCompile(` - \d+$`)

// The "raw (readable)" number forms produced by PlainValue. Compact layouts
// keep only the readable half.
var (
	groupedInteger  = regexp.MustCompile(`\b(-?\d{5,}) \((-?[\d,]+)\)`)
	tokenAmount     = regexp.MustCompile(`\b(\d+) \((-?\d+(?:\.\d+)?)( [^\s()]+)?\)`)
	unlimitedAmount = regexp.MustCompile(`\b(\d+) \((uint\d+\.max(?: \(∞\))?), all ([^\s()]+)\)`)
	timestampValue  = regexp.MustCompile(`\b(\d{9,10}) \((\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} UTC, [^)]+)\)`)
)

// compactNumberText rewrites every "raw (readable)" integer and token amount
// in text into its readable form only: "1000000000 (1,000,000,000)" becomes
// "1,000,000,000", "1000000 (1 USDC)" becomes "1 USDC" and a timestamp
// becomes its date.
func compactNumberText(text string) string {
	text = timestampValue.ReplaceAllString(text, "$2")
	text = unlimitedAmount.ReplaceAllString(text, "$2 $3")
	text = groupedInteger.ReplaceAllString(text, "$2")
	return tokenAmount.ReplaceAllStringFunc(text, func(m string) string {
		sub := tokenAmount.FindStringSubmatch(m)
		return jarviscommon.CompactAmount(sub[2]) + sub[3]
	})
}

// compactAddressText rewrites every "0x… (Desc)" occurrence in text into the
// NameFirst / ShortAddress form. Text without addresses is returned unchanged.
func compactAddressText(text string) string {
	// Two passes: adjacent addresses separated by a single character share
	// that character as both the trailing and leading boundary, so the first
	// pass consumes it and skips the second address.
	for pass := 0; pass < 2; pass++ {
		text = addressWithDesc.ReplaceAllStringFunc(text, func(m string) string {
			sub := addressWithDesc.FindStringSubmatch(m)
			desc := descDecimalSuffix.ReplaceAllString(sub[4], "")
			return sub[1] + jarviscommon.NameFirst(jarviscommon.Address{Address: sub[2], Desc: desc}, false) + sub[5]
		})
	}
	return text
}

// compactHex shortens a bare, long hex blob to its ends plus a byte count.
func compactHex(text string) string {
	if len(text) <= longHexLen || !strings.HasPrefix(text, "0x") || strings.ContainsAny(text, " (") {
		return text
	}
	return fmt.Sprintf("%s…%s (%d bytes)", text[:6], text[len(text)-4:], (len(text)-2)/2)
}

// txPrinter carries the per-render choices so the helpers below stay short.
type txPrinter struct {
	u       ui.UI
	layout  TxLayout
	compact bool // shorten addresses / hex, collapse long arrays
	// width is the terminal column count used to wrap event lines; 0 disables
	// wrapping (non-TTY output, tests).
	width int
	// expandAll keeps every array element visible even in compact mode; used
	// when echoing user input, where hiding elements would hide typos.
	expandAll bool
}

// NewParamDisplay builds the view-model for one decoded parameter without
// printing it.
func NewParamDisplay(param jarviscommon.ParamResult) ParamDisplay {
	return buildParamDisplay(param)
}

// ParamValueLines renders the value of a decoded parameter for echoing back
// what jarvis understood from user input. The first line is the value itself
// (or an item count for tuples/arrays); following lines expand the structure
// as a tree. Addresses are shortened name-first, nothing is collapsed.
func ParamValueLines(u ui.UI, d ParamDisplay) []string {
	p := txPrinter{u: u, compact: true, expandAll: true}
	return p.valueTree(d)
}

// valueTree is paramTree without the leading name/type: the caller has
// already labelled the parameter.
func (p txPrinter) valueTree(d ParamDisplay) []string {
	switch {
	case d.Values != nil && len(d.Values) == 1:
		return []string{p.text(d.Values[0])}
	case d.Values != nil:
		var kids [][]string
		for _, v := range d.Values {
			kids = append(kids, []string{p.text(v)})
		}
		return append([]string{p.muted(fmt.Sprintf("[%d items]", len(d.Values)))}, children(kids)...)
	case len(d.Tuples) == 1:
		return append([]string{p.muted("tuple")}, children(p.fieldLines(d.Tuples[0].Fields))...)
	case d.Tuples != nil:
		var kids [][]string
		for i, t := range d.Tuples {
			kid := []string{p.muted(fmt.Sprintf("[%d]", i))}
			kid = append(kid, children(p.fieldLines(t.Fields))...)
			kids = append(kids, kid)
		}
		return append([]string{p.muted(fmt.Sprintf("[%d items]", len(d.Tuples)))}, children(kids)...)
	case d.Arrays != nil:
		var kids [][]string
		for _, elem := range d.Arrays {
			kids = append(kids, p.valueTree(elem))
		}
		return append([]string{p.muted(fmt.Sprintf("[%d items]", len(d.Arrays)))}, children(kids)...)
	}
	// An empty array/slice has no Values, Tuples or Arrays at all.
	return []string{p.muted("[0 items]")}
}

func (p txPrinter) text(st ui.StyledText) string {
	sev := st.Severity
	// Green for "in the address book" earns its keep on the signing card; in
	// a dense read-only view it just competes with the status.
	if p.compact && sev == ui.SeveritySuccess {
		sev = ui.SeverityInfo
	}
	return p.u.Style(ui.StyledText{Text: p.plainText(st), Severity: sev})
}

// plainText is text without the colour: what the row will measure as.
func (p txPrinter) plainText(st ui.StyledText) string {
	if p.compact {
		return compactHex(compactNumberText(compactAddressText(st.Text)))
	}
	return st.Text
}

func (p txPrinter) muted(s string) string {
	return p.u.Style(ui.StyledText{Text: s, Severity: ui.SeverityMuted})
}

func (p txPrinter) bold(s string) string {
	return p.u.Style(ui.StyledText{Text: s, Severity: ui.SeverityCritical})
}

func statusText(status string) ui.StyledText {
	switch status {
	case "done":
		return ui.StyledText{Text: "✓ done", Severity: ui.SeveritySuccess}
	case "reverted":
		return ui.StyledText{Text: "✗ reverted", Severity: ui.SeverityError}
	case "pending":
		return ui.StyledText{Text: "… pending", Severity: ui.SeverityWarn}
	case "lost":
		return ui.StyledText{Text: "✗ lost", Severity: ui.SeverityError}
	default:
		return ui.StyledText{Text: status, Severity: ui.SeverityInfo}
	}
}

// intent is the short description of what the tx did: the method name for a
// contract call, "transfer <value>" for a plain value transfer.
func (p txPrinter) intent(d *TxDisplay, network networks.Network) string {
	switch {
	case d.TxType == "normal":
		return "transfer " + d.Value + " " + network.GetNativeTokenSymbol()
	case d.TxType == "contract creation":
		return "deploy contract"
	case d.FunctionCall == nil:
		return "contract call"
	case d.FunctionCall.Method == "" && (d.FunctionCall.Data == "" || d.FunctionCall.Data == "0x"):
		return "transfer " + d.Value + " " + network.GetNativeTokenSymbol() + " to contract"
	case d.FunctionCall.Method == "":
		return "<undecoded call>"
	default:
		return d.FunctionCall.Method
	}
}

func (p txPrinter) headline(d *TxDisplay, network networks.Network) string {
	return fmt.Sprintf("%s   %s  →  %s",
		p.u.Style(statusText(d.Status)),
		p.bold(p.intent(d, network)),
		p.text(d.To),
	)
}

// printTxDisplay renders d according to layout. The order is by importance:
// what happened (headline), what moved (transfers), the ERC-7730 clear-signed
// reading when a descriptor matched, what was called, what was emitted, and
// the headline again so the last line on screen is the summary.
func printTxDisplay(u ui.UI, d *TxDisplay, network networks.Network, layout TxLayout) {
	p := txPrinter{u: u, layout: layout, compact: layout != LayoutInfoFull, width: ui.TerminalWidth()}

	if d.TxType == "" {
		u.Info("%s", p.headline(d, network))
		u.Error("Checking tx type failed: %s", d.Error)
		return
	}

	u.Info("%s", p.headline(d, network))
	indent := strings.Repeat(" ", ui.VisibleWidth(statusText(d.Status).Text)+3)
	u.Info("%s", p.muted(indent+p.details(d, network)))
	if d.RevertReason != "" {
		u.Info("%s%s", indent, p.u.Style(ui.StyledText{Text: "reason  " + d.RevertReason, Severity: ui.SeverityError}))
	}

	if layout == LayoutInfoFull {
		p.printCard(d, network)
	}
	if d.TxType == "normal" {
		return
	}

	printed := false
	if len(d.NetEffect) > 0 {
		p.printNetEffect(d.NetEffect)
		printed = true
	}
	if len(d.Transfers) > 0 {
		p.printTransfers(d.Transfers)
		printed = true
	}
	emptyCall := d.FunctionCall != nil && d.FunctionCall.Method == "" &&
		(d.FunctionCall.Data == "" || d.FunctionCall.Data == "0x") && len(d.FunctionCall.InnerCalls) == 0
	showCall := d.FunctionCall != nil && !emptyCall && (layout != LayoutPostSign || d.Status == "reverted")
	// Post-sign already showed the panel on the signing card. Info layouts
	// print it above the ABI call so the operator gets the same reading
	// without signing. Leading Info("") matches Subsection spacing; a
	// no-op callback then looks the same as today's transfers → call gap.
	if layout != LayoutPostSign && d.ClearSign != nil {
		u.Info("")
		d.ClearSign(u)
	}
	if showCall {
		p.printCall(d.FunctionCall)
		printed = true
	}
	if len(d.Logs) > 0 {
		if layout == LayoutInfoFull {
			printAllLogs(u, d.Logs)
		} else {
			p.printEvents(d.Logs)
		}
		printed = true
	}
	if d.Error != "" {
		u.Warn("! %s", d.Error)
	}
	if printed && layout != LayoutPostSign {
		u.Info("")
		footer := p.headline(d, network)
		if d.Hash != "" {
			footer += "   " + p.muted(jarviscommon.ShortAddress(d.Hash))
		}
		u.Info("%s", footer)
	}
}

// details is the second headline line: the numbers that qualify the tx.
func (p txPrinter) details(d *TxDisplay, network networks.Network) string {
	sym := network.GetNativeTokenSymbol()
	from := d.From.Text
	if p.compact {
		from = compactAddressText(from)
	}
	parts := []string{"from " + from}
	if p.layout != LayoutPostSign {
		parts = []string{network.GetName(), "from " + from}
	}
	if d.TxType != "normal" && d.Value != "" {
		parts = append(parts, "value "+d.Value+" "+sym)
	}
	if d.GasCost != "" {
		gas := "gas " + d.GasCost + " " + sym
		if p.layout == LayoutPostSign && d.GasUsed != "" && d.GasLimit != "" {
			gas += fmt.Sprintf(" (%s / %s)", d.GasUsed, d.GasLimit)
		}
		parts = append(parts, gas)
	}
	if d.Nonce != "" {
		parts = append(parts, "nonce "+d.Nonce)
	}
	if d.BlockNumber != "" {
		parts = append(parts, "block "+d.BlockNumber)
	}
	return strings.Join(parts, "   ")
}

// printCard is the full key/value summary shown with -x.
func (p txPrinter) printCard(d *TxDisplay, network networks.Network) {
	label := func(s string) ui.TableCell { return ui.TCS(s, ui.SeverityMuted) }
	rows := [][2]ui.TableCell{}
	if d.Hash != "" {
		rows = append(rows, [2]ui.TableCell{label("Hash"), ui.TC(d.Hash)})
	}
	st := statusText(d.Status)
	rows = append(rows,
		[2]ui.TableCell{label("Status"), ui.TCS(st.Text, st.Severity)},
		[2]ui.TableCell{label("From"), tableCell(d.From)},
		[2]ui.TableCell{label("To"), tableCell(d.To)},
		[2]ui.TableCell{label("Value"), ui.TC(d.Value + " " + network.GetNativeTokenSymbol())},
	)
	if d.Nonce != "" {
		rows = append(rows,
			[2]ui.TableCell{label("Nonce"), ui.TC(d.Nonce)},
			[2]ui.TableCell{label("Gas price"), ui.TC(d.GasPrice + " gwei")},
			[2]ui.TableCell{label("Gas limit"), ui.TC(d.GasLimit)},
			[2]ui.TableCell{label("Gas used"), ui.TC(d.GasUsed)},
			[2]ui.TableCell{label("Gas cost"), ui.TC(d.GasCost + " " + network.GetNativeTokenSymbol())},
		)
	}
	if d.BlockNumber != "" {
		rows = append(rows, [2]ui.TableCell{label("Block"), ui.TC(d.BlockNumber)})
	}
	p.u.Info("")
	p.u.KeyValueCells(rows)
}

// maxCompactTransfers caps the Transfers list in compact layouts. Past this
// the Net effect block carries the meaning; -x still lists everything.
const maxCompactTransfers = 8

// printNetEffect renders the per-address summary: one line per address, its
// token deltas coloured by sign.
func (p txPrinter) printNetEffect(rows []NetEffectDisplay) {
	p.u.Subsection("Net effect")
	addrWidth := 0
	plainAddr := func(r NetEffectDisplay) string {
		if p.compact {
			return compactAddressText(r.Address.Text)
		}
		return r.Address.Text
	}
	for _, r := range rows {
		if w := ui.VisibleWidth(plainAddr(r)); w > addrWidth {
			addrWidth = w
		}
	}
	for _, r := range rows {
		pad := strings.Repeat(" ", addrWidth-ui.VisibleWidth(plainAddr(r)))
		deltas := make([]string, len(r.Deltas))
		for i, delta := range r.Deltas {
			text := delta
			if p.compact {
				amount, sym, _ := strings.Cut(delta, " ")
				text = string(amount[0]) + jarviscommon.CompactAmount(amount[1:])
				if sym != "" {
					text += " " + compactAddressText(sym)
				}
			}
			sev := ui.SeveritySuccess
			if strings.HasPrefix(delta, "-") {
				sev = ui.SeverityError
			}
			deltas[i] = p.u.Style(ui.StyledText{Text: text, Severity: sev})
		}
		p.u.Indent().Info("%s%s   %s", p.text(r.Address), pad, strings.Join(deltas, "   "))
	}
}

func (p txPrinter) printTransfers(ts []TransferDisplay) {
	title := "Transfers"
	hidden := 0
	if p.compact && len(ts) > maxCompactTransfers {
		title = fmt.Sprintf("Transfers (%d)", len(ts))
		hidden = len(ts) - maxCompactTransfers
		ts = ts[:maxCompactTransfers]
	}
	p.u.Subsection(title)
	amountWidth := 0
	amountText := func(t TransferDisplay) string {
		if p.compact && !strings.HasPrefix(t.Amount, "#") {
			return jarviscommon.CompactAmount(t.Amount)
		}
		return t.Amount
	}
	plain := func(t TransferDisplay) string {
		if t.Unlimited {
			return "UNLIMITED " + p.plainText(t.Token)
		}
		return amountText(t) + " " + p.plainText(t.Token)
	}
	// Every row is "amount   from  verb  to" with each column padded to the
	// widest entry so the arrows and destinations line up. Deposits and
	// withdrawals name the mechanism in place of the missing party; approvals
	// swap the arrow for a verb, and an unlimited allowance is flagged in the
	// amount column where the eye already is.
	type cell struct{ plain, styled string }
	fromCell := func(t TransferDisplay) cell {
		if t.Kind == "deposit" {
			return cell{"deposit", p.muted("deposit")}
		}
		return cell{p.plainText(t.From), p.text(t.From)}
	}
	toCell := func(t TransferDisplay) cell {
		if t.Kind == "withdrawal" {
			return cell{"withdrawal", p.muted("withdrawal")}
		}
		return cell{p.plainText(t.To), p.text(t.To)}
	}
	verbCell := func(t TransferDisplay) cell {
		if t.Kind == "approval" {
			return cell{"approves", "approves"}
		}
		return cell{"→", "→"}
	}
	fromWidth, verbWidth := 0, 0
	for _, t := range ts {
		if w := ui.VisibleWidth(plain(t)); w > amountWidth {
			amountWidth = w
		}
		if w := ui.VisibleWidth(fromCell(t).plain); w > fromWidth {
			fromWidth = w
		}
		if w := ui.VisibleWidth(verbCell(t).plain); w > verbWidth {
			verbWidth = w
		}
	}
	for _, t := range ts {
		amount := amountText(t) + " " + p.text(t.Token)
		if t.Unlimited {
			amount = p.u.Style(ui.StyledText{Text: plain(t), Severity: ui.SeverityWarn})
		}
		from, verb, to := fromCell(t), verbCell(t), toCell(t)
		p.u.Indent().Info("%s%s   %s%s  %s%s  %s",
			amount, strings.Repeat(" ", amountWidth-ui.VisibleWidth(plain(t))),
			from.styled, strings.Repeat(" ", fromWidth-ui.VisibleWidth(from.plain)),
			verb.styled, strings.Repeat(" ", verbWidth-ui.VisibleWidth(verb.plain)),
			to.styled)
	}
	if hidden > 0 {
		p.u.Indent().Info("%s", p.muted(fmt.Sprintf("… %d more (-x lists all)", hidden)))
	}
}

// printCall renders the top-level call and its inner calls as an indented
// tree instead of bordered tables.
func (p txPrinter) printCall(d *FunctionCallDisplay) {
	if d.Method == "" {
		p.u.Subsection("Call  <undecoded>  →  " + p.text(d.Destination))
	} else {
		p.u.Subsection("Call  " + d.Method + "  →  " + p.text(d.Destination))
	}
	p.printCallBody(p.u.Indent(), d)
}

func (p txPrinter) printCallBody(u ui.UI, d *FunctionCallDisplay) {
	if d.Value != "" {
		u.Info("%s %s", p.muted("value"), d.Value)
	}
	if d.Error != "" {
		u.Warn("! %s", friendlyDecodeError(d.Error))
	}
	if d.Method == "" && d.Data != "" {
		selector := d.Data
		if len(selector) > 10 {
			selector = selector[:10]
		}
		u.Info("%s %s   %s", p.muted("selector"), selector, p.muted(fmt.Sprintf("%d bytes", (len(d.Data)-2)/2)))
		if !p.compact {
			printRawCalldata(u, d.Data)
		}
	}
	for _, line := range p.paramLines(d.Params) {
		u.Info("%s", line)
	}
	for _, inner := range d.InnerCalls {
		method := inner.Method
		if method == "" {
			method = "<undecoded>"
		}
		u.Info("↳ %s  →  %s", p.bold(method), p.text(inner.Destination))
		p.printCallBody(u.Indent(), inner)
	}
}

// paramLines renders a parameter list as aligned "name  value  type" lines,
// with tuples and arrays expanded underneath as a tree.
func (p txPrinter) paramLines(params []ParamDisplay) []string {
	nameWidth := 0
	for _, prm := range params {
		if w := ui.VisibleWidth(prm.Name); w > nameWidth {
			nameWidth = w
		}
	}
	var lines []string
	for _, prm := range params {
		lines = append(lines, p.paramTree(prm, nameWidth)...)
	}
	return lines
}

func (p txPrinter) row(name string, nameWidth int, value, typ string) string {
	pad := strings.Repeat(" ", nameWidth-ui.VisibleWidth(name))
	if value == "" {
		return fmt.Sprintf("%s%s  %s", name, pad, p.muted(typ))
	}
	return fmt.Sprintf("%s%s  %s  %s", name, pad, value, p.muted(typ))
}

// children prefixes each child's lines with tree glyphs: the first line gets a
// branch, continuation lines get a guide, so nested structures stay legible.
func children(lines [][]string) []string {
	var out []string
	for i, child := range lines {
		last := i == len(lines)-1
		for j, l := range child {
			if j == 0 {
				out = append(out, ui.TreeBranch(last)+l)
			} else {
				out = append(out, ui.TreeGuide(last)+l)
			}
		}
	}
	return out
}

func (p txPrinter) paramTree(d ParamDisplay, nameWidth int) []string {
	collapse := func(n int) bool { return p.compact && !p.expandAll && n > collapseAbove }
	summary := func(n int) string {
		s := p.muted(fmt.Sprintf("[%d items]", n))
		if collapse(n) {
			s += p.muted("  (-x to expand)")
		}
		return s
	}

	switch {
	case d.Values != nil && len(d.Values) == 1:
		return []string{p.row(d.Name, nameWidth, p.text(d.Values[0]), d.Type)}

	case d.Values != nil:
		lines := []string{p.row(d.Name, nameWidth, summary(len(d.Values)), d.Type)}
		if collapse(len(d.Values)) {
			return lines
		}
		var kids [][]string
		for _, v := range d.Values {
			kids = append(kids, []string{p.text(v)})
		}
		return append(lines, children(kids)...)

	case len(d.Tuples) == 1:
		lines := []string{p.row(d.Name, nameWidth, "", d.Type)}
		return append(lines, children(p.fieldLines(d.Tuples[0].Fields))...)

	case d.Tuples != nil:
		lines := []string{p.row(d.Name, nameWidth, summary(len(d.Tuples)), d.Type)}
		if collapse(len(d.Tuples)) {
			return lines
		}
		var kids [][]string
		for i, t := range d.Tuples {
			kid := []string{p.muted(fmt.Sprintf("[%d]", i))}
			kid = append(kid, children(p.fieldLines(t.Fields))...)
			kids = append(kids, kid)
		}
		return append(lines, children(kids)...)

	case d.Arrays != nil:
		lines := []string{p.row(d.Name, nameWidth, summary(len(d.Arrays)), d.Type)}
		if collapse(len(d.Arrays)) {
			return lines
		}
		var kids [][]string
		for _, elem := range d.Arrays {
			kids = append(kids, p.paramTree(elem, ui.VisibleWidth(elem.Name)))
		}
		return append(lines, children(kids)...)
	}
	return []string{p.row(d.Name, nameWidth, p.muted("[0 items]"), d.Type)}
}

func (p txPrinter) fieldLines(fields []ParamDisplay) [][]string {
	nameWidth := 0
	for _, f := range fields {
		if w := ui.VisibleWidth(f.Name); w > nameWidth {
			nameWidth = w
		}
	}
	var out [][]string
	for _, f := range fields {
		out = append(out, p.paramTree(f, nameWidth))
	}
	return out
}

// printEvents renders one line per event: "N. Name  emitter   arg value  arg value".
// undecodedEventLabel stands in for the name of a log no ABI describes.
const undecodedEventLabel = "<undecoded>"

func (p txPrinter) printEvents(logs []LogDisplay) {
	p.u.Subsection(fmt.Sprintf("Events (%d)", len(logs)))
	nameWidth := 0
	undecoded := 0
	for _, l := range logs {
		if l.Name == "" {
			undecoded++
		}
		if w := ui.VisibleWidth(eventName(l)); w > nameWidth {
			nameWidth = w
		}
	}
	for i, l := range logs {
		var args []string
		for _, t := range l.Topics {
			args = append(args, p.muted(t.Name)+" "+p.text(t.Verbose))
		}
		for _, prm := range l.Data {
			args = append(args, p.muted(prm.Name)+" "+p.paramInline(prm))
		}
		plain := eventName(l)
		name := plain + strings.Repeat(" ", nameWidth-ui.VisibleWidth(plain))
		if l.Name == "" {
			name = p.muted(name)
		} else {
			name = p.bold(name)
		}
		head := fmt.Sprintf("%d. %s  %s", i+1, name, p.text(l.Address))
		// Continuation lines start under the address so the event name and
		// number column stay scannable.
		contIndent := len(fmt.Sprintf("%d. ", len(logs))) + nameWidth + 2
		for _, line := range p.packArgs(head, args, contIndent, 2) {
			p.u.Indent().Info("%s", line)
		}
	}
	if undecoded > 0 {
		p.u.Indent().Info("%s", p.muted(fmt.Sprintf(
			"%d event(s) shown raw: the emitting contract has no ABI available (unverified or explorer unreachable)", undecoded)))
	}
}

// packArgs lays out args after head, greedily filling terminal lines. The
// first line holds head plus as many args as fit; overflow lines are indented
// by contIndent. baseIndent is the indentation the caller will add. With no
// known width everything goes on one line.
func (p txPrinter) packArgs(head string, args []string, contIndent, baseIndent int) []string {
	if len(args) == 0 {
		return []string{head}
	}
	avail := p.width - baseIndent
	joined := head + "   " + strings.Join(args, "  ")
	if p.width == 0 || ui.VisibleWidth(joined) <= avail {
		return []string{joined}
	}
	lines := []string{}
	cur, curW := head+" ", ui.VisibleWidth(head)+1
	first := true
	pad := strings.Repeat(" ", contIndent)
	for _, a := range args {
		w := ui.VisibleWidth(a)
		if !first && curW+2+w > avail {
			lines = append(lines, cur)
			cur, curW = pad+a, contIndent+w
			continue
		}
		cur += "  " + a
		curW += 2 + w
		first = false
	}
	return append(lines, cur)
}

func eventName(l LogDisplay) string {
	if l.Name == "" {
		return undecodedEventLabel
	}
	return l.Name
}

// friendlyDecodeError rewrites the analyzer's calldata error into a sentence
// that says what it means for the operator, keeping the technical cause.
func friendlyDecodeError(err string) string {
	cause := strings.TrimPrefix(err, "couldn't decode calldata: ")
	switch {
	case strings.Contains(cause, "no method with id"):
		sel := cause[strings.LastIndex(cause, " ")+1:]
		return fmt.Sprintf("calldata not decoded: no available ABI covers selector %s (contract unverified or ABI mismatch)", sel)
	case strings.HasPrefix(cause, "abi:"):
		return "calldata not decoded: it does not match the contract ABI (" + strings.TrimSpace(strings.TrimPrefix(cause, "abi:")) + ")"
	default:
		return "calldata not decoded: " + cause
	}
}

// paramInline is the single-line form of a parameter used in event lines.
func (p txPrinter) paramInline(d ParamDisplay) string {
	switch {
	case d.Values != nil && len(d.Values) == 1:
		return p.text(d.Values[0])
	case d.Values != nil:
		if p.compact && len(d.Values) > collapseAbove {
			return p.muted(fmt.Sprintf("[%d items]", len(d.Values)))
		}
		parts := make([]string, len(d.Values))
		for i, v := range d.Values {
			parts[i] = p.text(v)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case len(d.Tuples) > 0:
		return p.muted(fmt.Sprintf("[%d items]", len(d.Tuples)))
	case d.Arrays != nil:
		return p.muted(fmt.Sprintf("[%d items]", len(d.Arrays)))
	}
	return ""
}
