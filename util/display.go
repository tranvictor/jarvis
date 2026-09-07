package util

import (
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
)

// ── Severity helpers ─────────────────────────────────────────────────────────

// StyledAddress wraps the *target* of a call (tx `to`, inner-call destination)
// in a StyledText. Known addresses are Success (green); an unknown target is
// Warn (yellow) because sending a call somewhere the address book has never
// seen is the one address-level fact worth the reader's attention.
func StyledAddress(addr jarviscommon.Address) ui.StyledText {
	// The token's decimal count is analyzer bookkeeping, not something the
	// signer needs to read next to the destination.
	addr.Decimal = 0
	text := jarviscommon.PlainAddress(addr)
	if !jarviscommon.IsKnownAddress(addr) {
		return ui.StyledText{Text: text, Severity: ui.SeverityWarn}
	}
	return ui.StyledText{Text: text, Severity: ui.SeveritySuccess}
}

// styledParamAddress wraps an address that appears as data (a parameter, an
// event argument, the sender, a log emitter). Known addresses are green;
// unknown ones stay plain — most addresses in a busy tx are unknown, and
// colouring them all yellow would drown the warnings that matter.
func styledParamAddress(addr jarviscommon.Address) ui.StyledText {
	text := jarviscommon.PlainAddress(addr)
	if !jarviscommon.IsKnownAddress(addr) {
		return ui.StyledText{Text: text, Severity: ui.SeverityInfo}
	}
	return ui.StyledText{Text: text, Severity: ui.SeveritySuccess}
}

// styledValue wraps a common.Value in a StyledText.
// Address values inherit their severity from styledParamAddress; all other
// values are SeverityInfo (plain).
func styledValue(v jarviscommon.Value) ui.StyledText {
	if v.Kind == jarviscommon.DisplayAddress && v.Address != nil {
		return styledParamAddress(*v.Address)
	}
	return ui.StyledText{Text: jarviscommon.PlainValue(v), Severity: ui.SeverityInfo}
}

func tableCell(st ui.StyledText) ui.TableCell {
	return ui.TCS(st.Text, st.Severity)
}

// ── Build phase (pure: no UI side-effects) ──────────────────────────────────

func buildParamDisplay(param jarviscommon.ParamResult) ParamDisplay {
	d := ParamDisplay{Name: param.Name, Type: param.Type}
	switch {
	case param.Values != nil:
		for _, v := range param.Values {
			d.Values = append(d.Values, styledValue(v))
		}
	case param.Tuples != nil:
		for _, tuple := range param.Tuples {
			td := TupleDisplay{Name: tuple.Name, Type: tuple.Type}
			for _, field := range tuple.Values {
				td.Fields = append(td.Fields, buildParamDisplay(field))
			}
			d.Tuples = append(d.Tuples, td)
		}
	case param.Arrays != nil:
		for _, arr := range param.Arrays {
			d.Arrays = append(d.Arrays, buildParamDisplay(arr))
		}
	}
	return d
}

func buildFunctionCallDisplay(fc *jarviscommon.FunctionCall, nested bool) *FunctionCallDisplay {
	d := &FunctionCallDisplay{
		Destination: StyledAddress(fc.Destination),
		Error:       fc.Error,
		Method:      fc.Method,
	}
	if nested && fc.Value != nil {
		d.Value = fmt.Sprintf("%f ETH", jarviscommon.BigToFloat(fc.Value, 18))
	}
	// Only carried when the method couldn't be resolved — a decoded call already
	// shows everything the calldata contains.
	if fc.Method == "" && len(fc.Data) > 0 {
		d.Data = hexutil.Encode(fc.Data)
	}
	for _, param := range fc.Params {
		d.Params = append(d.Params, buildParamDisplay(param))
	}
	for _, inner := range fc.DecodedFunctionCalls {
		d.InnerCalls = append(d.InnerCalls, buildFunctionCallDisplay(inner, true))
	}
	return d
}

func buildLogDisplay(log jarviscommon.LogResult) LogDisplay {
	d := LogDisplay{Name: log.Name}
	if log.Address.Address != "" {
		d.Address = styledParamAddress(log.Address)
	}
	for _, topic := range log.Topics {
		d.Topics = append(d.Topics, TopicDisplay{
			Name:    topic.Name,
			Verbose: styledValue(topic.Value),
		})
	}
	for _, param := range log.Data {
		d.Data = append(d.Data, buildParamDisplay(param))
	}
	return d
}

func buildTxDisplay(result *jarviscommon.TxResult) *TxDisplay {
	d := &TxDisplay{
		Status:       result.Status,
		From:         styledParamAddress(result.From),
		To:           StyledAddress(result.To),
		Value:        result.Value,
		TxType:       result.TxType,
		RevertReason: result.RevertReason,
		Error:        result.Error,
		Nonce:        result.Nonce,
		GasPrice:     result.GasPrice,
		GasLimit:     result.GasLimit,
		GasUsed:      result.GasUsed,
		GasCost:      result.GasCost,
		BlockNumber:  result.BlockNumber,
	}
	if result.TxType == "" || result.TxType == "normal" {
		return d
	}
	if result.FunctionCall != nil {
		d.FunctionCall = buildFunctionCallDisplay(result.FunctionCall, false)
	}
	d.Transfers = buildTransfers(result.Logs)
	d.NetEffect = buildNetEffect(d.Transfers, d.From)
	for _, l := range result.Logs {
		d.Logs = append(d.Logs, buildLogDisplay(l))
	}
	return d
}

// buildTransfers derives asset movements from the well-known token events so
// the reader gets "what moved" without scanning the event table.
func buildTransfers(logs []jarviscommon.LogResult) []TransferDisplay {
	var out []TransferDisplay
	for _, l := range logs {
		args := map[string]jarviscommon.Value{}
		for _, t := range l.Topics {
			args[strings.ToLower(t.Name)] = t.Value
		}
		for _, p := range l.Data {
			if len(p.Values) == 1 {
				args[strings.ToLower(p.Name)] = p.Values[0]
			}
		}
		amount, unlimited, ok := transferAmount(args)
		if !ok {
			continue
		}
		td := TransferDisplay{Amount: amount, Unlimited: unlimited, Token: transferToken(l, args)}
		// Parties are looked up by the usual names first and by topic
		// position otherwise: DAI/WETH-style ABIs call them src/guy/wad, and
		// any ERC-20 puts (from, to) / (owner, spender) in topics 1 and 2.
		party := func(idx int, names ...string) ui.StyledText {
			if v := transferParty(args, names...); v.Text != "" {
				return v
			}
			if idx < len(l.Topics) {
				return styledValue(l.Topics[idx].Value)
			}
			return ui.StyledText{}
		}
		switch l.Name {
		case "Transfer":
			td.Kind = "transfer"
			td.From, td.To = party(0, "from", "src", "_from"), party(1, "to", "dst", "_to")
		case "Approval":
			td.Kind = "approval"
			td.From, td.To = party(0, "owner", "src", "_owner"), party(1, "spender", "guy", "_spender")
		case "Deposit":
			td.Kind = "deposit"
			td.To = party(0, "dst", "to", "user", "account")
		case "Withdrawal":
			td.Kind = "withdrawal"
			td.From = party(0, "src", "from", "user", "account")
		default:
			continue
		}
		out = append(out, td)
	}
	return out
}

// netEffectMinTransfers is the transfer count from which a per-address net
// summary is worth printing; below it the list itself is the summary.
const netEffectMinTransfers = 4

// netEffectMaxRows caps the summary so it stays a summary.
const netEffectMaxRows = 6

// buildNetEffect folds transfers into per-address, per-token net changes.
// Approvals are not movements and NFTs are counted as ±1. The sender comes
// first; other addresses follow by how many tokens they touched. Mint/burn
// counterparties (the zero address) are left out.
func buildNetEffect(transfers []TransferDisplay, sender ui.StyledText) []NetEffectDisplay {
	if len(transfers) < netEffectMinTransfers {
		return nil
	}
	type tokenNet struct {
		token ui.StyledText
		net   *big.Rat
	}
	type addrNet struct {
		addr   ui.StyledText
		tokens []*tokenNet // insertion order
		byName map[string]*tokenNet
	}
	addrs := map[string]*addrNet{}
	order := []string{}
	add := func(who ui.StyledText, token ui.StyledText, amount *big.Rat) {
		if who.Text == "" || jarviscommon.IsZeroAddress(strings.Fields(who.Text)[0]) {
			return
		}
		an := addrs[who.Text]
		if an == nil {
			an = &addrNet{addr: who, byName: map[string]*tokenNet{}}
			addrs[who.Text] = an
			order = append(order, who.Text)
		}
		tn := an.byName[token.Text]
		if tn == nil {
			tn = &tokenNet{token: token, net: new(big.Rat)}
			an.byName[token.Text] = tn
			an.tokens = append(an.tokens, tn)
		}
		tn.net.Add(tn.net, amount)
	}
	for _, t := range transfers {
		amount := new(big.Rat)
		if strings.HasPrefix(t.Amount, "#") {
			amount.SetInt64(1)
		} else if _, ok := amount.SetString(t.Amount); !ok {
			continue
		}
		neg := new(big.Rat).Neg(amount)
		switch t.Kind {
		case "transfer":
			add(t.From, t.Token, neg)
			add(t.To, t.Token, amount)
		case "deposit":
			add(t.To, t.Token, amount)
		case "withdrawal":
			add(t.From, t.Token, neg)
		}
	}
	rows := []NetEffectDisplay{}
	for _, key := range order {
		an := addrs[key]
		row := NetEffectDisplay{Address: an.addr}
		for _, tn := range an.tokens {
			if tn.net.Sign() == 0 {
				continue
			}
			row.Deltas = append(row.Deltas, signedAmount(tn.net)+" "+tn.token.Text)
		}
		if len(row.Deltas) > 0 {
			rows = append(rows, row)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if (rows[i].Address.Text == sender.Text) != (rows[j].Address.Text == sender.Text) {
			return rows[i].Address.Text == sender.Text
		}
		return len(rows[i].Deltas) > len(rows[j].Deltas)
	})
	if len(rows) > netEffectMaxRows {
		rows = rows[:netEffectMaxRows]
	}
	return rows
}

// signedAmount renders r as a decimal with an explicit sign and no trailing
// zeros, exact to 18 places.
func signedAmount(r *big.Rat) string {
	s := r.FloatString(18)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if r.Sign() > 0 {
		return "+" + s
	}
	return s
}

// transferAmount picks the amount-like argument of a token event and renders
// it with the token's decimals when the analyzer attached a hint. The symbol
// is carried separately by TransferDisplay.Token.
func transferAmount(args map[string]jarviscommon.Value) (amount string, unlimited, ok bool) {
	if v, has := args["tokenid"]; has {
		return "#" + v.Raw, false, true
	}
	for _, name := range []string{"value", "amount", "wad", "_value", "_amount"} {
		v, has := args[name]
		if !has {
			continue
		}
		if _, isMax := jarviscommon.MaxUintLabel(v.Raw); isMax {
			return "unlimited", true, true
		}
		if v.Kind == jarviscommon.DisplayToken && v.Token != nil {
			return jarviscommon.BigToFloatString(jarviscommon.StringToBig(v.Raw), v.Token.Decimal), false, true
		}
		return v.Raw, false, true
	}
	return "", false, false
}

func transferToken(l jarviscommon.LogResult, args map[string]jarviscommon.Value) ui.StyledText {
	for _, v := range args {
		if v.Kind == jarviscommon.DisplayToken && v.Token != nil && v.Token.Symbol != "" {
			return ui.StyledText{Text: v.Token.Symbol, Severity: ui.SeveritySuccess}
		}
	}
	return styledParamAddress(l.Address)
}

func transferParty(args map[string]jarviscommon.Value, names ...string) ui.StyledText {
	for _, n := range names {
		if v, ok := args[n]; ok {
			return styledValue(v)
		}
	}
	return ui.StyledText{}
}

// ── Print phase (reads only from the display struct, colours via u.Style) ────

// flattenParamRows recursively converts a ParamDisplay into [label, value]
// rows. Complex types (tuples, arrays) are inlined with deeper indentation
// so that the entire parameter tree fits inside a single two-column table.
func flattenParamRows(d ParamDisplay, indent string) [][]ui.TableCell {
	label := indent + fmt.Sprintf("%s (%s)", d.Name, d.Type)

	// Scalar value(s).
	if d.Values != nil {
		if len(d.Values) == 1 {
			return [][]ui.TableCell{{ui.TC(label), tableCell(d.Values[0])}}
		}
		rows := make([][]ui.TableCell, len(d.Values))
		for i, v := range d.Values {
			rows[i] = []ui.TableCell{ui.TC(fmt.Sprintf("%s [%d]", label, i+1)), tableCell(v)}
		}
		return rows
	}

	// Single tuple: header row + indented children.
	if len(d.Tuples) == 1 {
		rows := [][]ui.TableCell{{ui.TC(label), ui.TC("")}}
		for _, field := range d.Tuples[0].Fields {
			rows = append(rows, flattenParamRows(field, indent+"  ")...)
		}
		return rows
	}

	// Multi-tuple (array of structs): header row + indexed children.
	if d.Tuples != nil {
		rows := [][]ui.TableCell{{ui.TC(label), ui.TC("")}}
		idxWidth := len(fmt.Sprintf("[%d]", len(d.Tuples)-1))
		for i, tuple := range d.Tuples {
			indexStr := fmt.Sprintf("[%d]", i)
			padded := indexStr + strings.Repeat(" ", idxWidth-len(indexStr))
			blank := strings.Repeat(" ", idxWidth)
			for j, field := range tuple.Fields {
				prefix := indent + "  " + blank + " "
				if j == 0 {
					prefix = indent + "  " + padded + " "
				}
				rows = append(rows, flattenParamRows(field, prefix)...)
			}
		}
		return rows
	}

	// Plain array.
	if d.Arrays != nil {
		rows := [][]ui.TableCell{{ui.TC(label), ui.TC("")}}
		for _, elem := range d.Arrays {
			rows = append(rows, flattenParamRows(elem, indent+"  ")...)
		}
		return rows
	}

	return nil
}

// printParamList renders a slice of ParamDisplays as a single grouped
// table. Consecutive scalar params share a group; each complex param
// (tuple / array) gets its own group.
func printParamList(u ui.UI, params []ParamDisplay) {
	var groups [][][]ui.TableCell
	var scalarGroup [][]ui.TableCell

	flushScalars := func() {
		if len(scalarGroup) > 0 {
			groups = append(groups, scalarGroup)
			scalarGroup = nil
		}
	}

	for _, p := range params {
		rows := flattenParamRows(p, "")
		if len(rows) == 0 {
			continue
		}
		if p.Values != nil {
			scalarGroup = append(scalarGroup, rows...)
		} else {
			flushScalars()
			groups = append(groups, rows)
		}
	}

	flushScalars()

	if len(groups) > 0 {
		u.PrintTable(&ui.Table{
			Headers: []string{"Parameter", "Value"},
			Groups:  groups,
		})
	}
}

// rawCalldataWidth is how many hex characters of raw calldata are printed per
// line. 64 keeps the line inside a standard terminal even a couple of indent
// levels deep, and lines up with 32-byte ABI words.
const rawCalldataWidth = 64

// printRawCalldata prints hex calldata as wrapped lines under a byte-count
// label. It stays outside any table on purpose: a multi-kilobyte blob in a
// table cell blows the box out past the terminal width and becomes unreadable,
// and truncating it is not an option on a signing path.
func printRawCalldata(u ui.UI, data string) {
	body := strings.TrimPrefix(data, "0x")
	u.Info("Raw calldata (%d bytes):", len(body)/2)
	uu := u.Indent()
	for i := 0; i < len(body); i += rawCalldataWidth {
		end := i + rawCalldataWidth
		if end > len(body) {
			end = len(body)
		}
		// "0x" on the first line only, continuation lines padded by two so the
		// hex columns still line up (and a copy-paste still reads as one blob).
		prefix := "  "
		if i == 0 {
			prefix = "0x"
		}
		uu.Info("%s%s", prefix, body[i:end])
	}
}

// logSimpleRows returns all simple [param, value] rows for a single log,
// used when building the combined all-logs table.
func logSimpleRows(d LogDisplay) [][]ui.TableCell {
	var rows [][]ui.TableCell
	for _, topic := range d.Topics {
		rows = append(rows, []ui.TableCell{ui.TC(topic.Name + " (indexed)"), tableCell(topic.Verbose)})
	}
	for _, param := range d.Data {
		rows = append(rows, flattenParamRows(param, "")...)
	}
	return rows
}

// printAllLogs renders all event logs as one unified 3-column table
// (Event | Parameter | Value). The event name appears only in the first row of
// each group; subsequent rows in the same log have an empty event cell.
func printAllLogs(u ui.UI, logs []LogDisplay) {
	if len(logs) == 0 {
		return
	}
	u.Section("Event Logs")

	groups := make([][][]ui.TableCell, len(logs))
	for i, d := range logs {
		eventLabel := fmt.Sprintf("%d. %s", i+1, eventName(d))
		paramRows := logSimpleRows(d)
		if len(paramRows) == 0 {
			groups[i] = [][]ui.TableCell{{ui.TC(eventLabel), ui.TC(""), ui.TC("")}}
			continue
		}
		group := make([][]ui.TableCell, len(paramRows))
		for j, pr := range paramRows {
			name := ui.TC("")
			if j == 0 {
				name = ui.TC(eventLabel)
			}
			group[j] = []ui.TableCell{name, pr[0], pr[1]}
		}
		groups[i] = group
	}
	u.PrintTable(&ui.Table{
		Headers: []string{"Event", "Parameter", "Value"},
		Groups:  groups,
	})
}

// ── Public API ───────────────────────────────────────────────────────────────

// DisplayParam builds the human-readable view-model for a single decoded ABI
// parameter and writes it to u via u.Style for correct terminal coloring.
func DisplayParam(u ui.UI, param jarviscommon.ParamResult) ParamDisplay {
	d := buildParamDisplay(param)
	printParamList(u, []ParamDisplay{d})
	return d
}

// DisplayParams builds view-models for a slice of ABI parameters and renders
// them together in one pass: all scalar params appear in a single table and
// complex params (tuples, arrays) are printed below it. This avoids the
// fragmented output produced by calling DisplayParam once per param.
func DisplayParams(u ui.UI, params []jarviscommon.ParamResult) []ParamDisplay {
	displays := make([]ParamDisplay, len(params))
	for i, p := range params {
		displays[i] = buildParamDisplay(p)
	}
	printParamList(u, displays)
	return displays
}

// DisplayFunctionCall builds the human-readable view-model for a decoded
// function call (and any recursively decoded inner calls) and writes it to u
// as an indented tree with full addresses and nothing collapsed — the form
// used wherever the reader is about to sign what they see.
func DisplayFunctionCall(u ui.UI, fc *jarviscommon.FunctionCall) *FunctionCallDisplay {
	d := buildFunctionCallDisplay(fc, false)
	PrintFunctionCall(u, d)
	return d
}

// NewFunctionCallDisplay builds the view-model for a decoded call without
// printing it.
func NewFunctionCallDisplay(fc *jarviscommon.FunctionCall) *FunctionCallDisplay {
	return buildFunctionCallDisplay(fc, false)
}

// PrintFunctionCall renders an already-built call view-model in full detail.
func PrintFunctionCall(u ui.UI, d *FunctionCallDisplay) {
	txPrinter{u: u, layout: LayoutInfoFull, compact: false}.printCall(d)
}

// DisplayTxResult builds the human-readable view-model for an analyzed
// transaction and writes it to u using the given layout. The returned
// *TxDisplay is always complete regardless of layout and serializes cleanly
// to JSON (StyledText fields marshal as plain strings).
//
// hash is the transaction hash shown in the headline/footer; pass an empty
// string to omit it (e.g. when the hash is already shown by the caller).
func DisplayTxResult(u ui.UI, result *jarviscommon.TxResult, network networks.Network, layout TxLayout, hash string) *TxDisplay {
	d := buildTxDisplay(result)
	d.Hash = hash
	printTxDisplay(u, d, network, layout)
	return d
}

// InfoLayout maps the --degen flag onto the `jarvis info` layouts.
func InfoLayout(degen bool) TxLayout {
	if degen {
		return LayoutInfoFull
	}
	return LayoutInfo
}

// PostSignLayout is the layout for a tx the user just signed: the delta view
// by default, everything with --degen.
func PostSignLayout(degen bool) TxLayout {
	if degen {
		return LayoutInfoFull
	}
	return LayoutPostSign
}
