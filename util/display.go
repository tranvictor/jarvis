package util

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
)

// ── Severity helpers ─────────────────────────────────────────────────────────

// StyledAddress wraps a common.Address in a StyledText.
// Known addresses (non-empty, non-"unknown" description) are Success (green);
// unknown ones are Warn (yellow) so they stand out without being alarming.
func StyledAddress(addr jarviscommon.Address) ui.StyledText {
	text := jarviscommon.PlainAddress(addr)
	if addr.Desc == "" || addr.Desc == "unknown" {
		return ui.StyledText{Text: text, Severity: ui.SeverityWarn}
	}
	return ui.StyledText{Text: text, Severity: ui.SeveritySuccess}
}

// styledValue wraps a common.Value in a StyledText.
// Address values inherit their severity from StyledAddress; all other values
// are SeverityInfo (plain).
func styledValue(v jarviscommon.Value) ui.StyledText {
	if v.Kind == jarviscommon.DisplayAddress && v.Address != nil {
		return StyledAddress(*v.Address)
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

func buildTxDisplay(result *jarviscommon.TxResult, fullDetail bool) *TxDisplay {
	d := &TxDisplay{
		Status: result.Status,
		From:   StyledAddress(result.From),
		To:     StyledAddress(result.To),
		Value:  result.Value,
		TxType: result.TxType,
		Error:  result.Error,
	}
	if fullDetail {
		d.Nonce = result.Nonce
		d.GasPrice = result.GasPrice
		d.GasLimit = result.GasLimit
		d.GasUsed = result.GasUsed
		d.GasCost = result.GasCost
	}
	if result.TxType == "" || result.TxType == "normal" {
		return d
	}
	if fullDetail && result.FunctionCall != nil {
		d.FunctionCall = buildFunctionCallDisplay(result.FunctionCall, false)
	}
	for _, l := range result.Logs {
		d.Logs = append(d.Logs, buildLogDisplay(l))
	}
	return d
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

// printParamList renders a slice of ParamDisplays as a single unified
// TableWithGroups. Consecutive scalar params share a group; each complex
// param (tuple / array) gets its own group.
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

// printUndecodedCall renders a call jarvis couldn't decode. Showing the
// destination, the selector and the raw calldata is what lets an operator tell
// *which* contract is missing an ABI (and inspect the payload by hand) instead
// of staring at a bare decode error — which matters most for one entry of a
// multisig batch, where the failing contract is otherwise invisible.
func printUndecodedCall(u ui.UI, d *FunctionCallDisplay, nested bool) {
	var rows [][]ui.TableCell
	if nested {
		// The arrow label already carries the destination; don't repeat it.
		u.Error("↳ <undecoded call>  [%s]", u.Style(d.Destination))
		u = u.Indent()
	} else {
		u.Section("Function call: <undecoded>")
		rows = append(rows, []ui.TableCell{ui.TC("Contract"), tableCell(d.Destination)})
	}

	if d.Value != "" {
		rows = append(rows, []ui.TableCell{ui.TC("Value"), ui.TC(d.Value)})
	}
	if len(d.Data) >= 10 {
		rows = append(rows, []ui.TableCell{ui.TC("Method ID"), ui.TC(d.Data[:10])})
	}
	if d.Error != "" {
		rows = append(
			rows,
			[]ui.TableCell{ui.TC("Error"), ui.TCS(d.Error, ui.SeverityError)},
		)
	}
	if len(rows) > 0 {
		u.PrintTable(&ui.Table{Groups: [][][]ui.TableCell{rows}})
	}
	if d.Data != "" {
		printRawCalldata(u, d.Data)
	}

	for _, inner := range d.InnerCalls {
		printFunctionCallDisplay(u.Indent(), inner, true)
	}
}

func printFunctionCallDisplay(u ui.UI, d *FunctionCallDisplay, nested bool) {
	if d.Method == "" {
		printUndecodedCall(u, d, nested)
		return
	}

	if nested {
		// Inner calls are visually subordinate — a simple arrow label, no Section.
		u.Info("↳ %s  [%s]", d.Method, u.Style(d.Destination))
		if d.Error != "" {
			u.Indent().Error("%s", d.Error)
		}
		printParamList(u.Indent(), d.Params)
		for _, inner := range d.InnerCalls {
			printFunctionCallDisplay(u.Indent(), inner, true)
		}
		return
	}

	u.Section(fmt.Sprintf("Function call: %s", d.Method))

	// Build a single TableWithGroups: contract metadata (group 0) + params (group 1).
	metaGroup := [][]ui.TableCell{{ui.TC("Contract"), tableCell(d.Destination)}}
	if d.Value != "" {
		metaGroup = append(metaGroup, []ui.TableCell{ui.TC("Value"), ui.TC(d.Value)})
	}
	// A method can resolve while its arguments fail to unpack; say so instead of
	// showing a name over an empty parameter list.
	if d.Error != "" {
		metaGroup = append(
			metaGroup,
			[]ui.TableCell{ui.TC("Error"), ui.TCS(d.Error, ui.SeverityError)},
		)
	}

	var paramGroup [][]ui.TableCell
	for _, p := range d.Params {
		paramGroup = append(paramGroup, flattenParamRows(p, "")...)
	}

	if len(paramGroup) > 0 {
		u.PrintTable(&ui.Table{Groups: [][][]ui.TableCell{metaGroup, paramGroup}})
	} else {
		u.PrintTable(&ui.Table{Groups: [][][]ui.TableCell{metaGroup}})
	}

	for _, inner := range d.InnerCalls {
		printFunctionCallDisplay(u.Indent(), inner, true)
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
		eventLabel := fmt.Sprintf("%d. %s", i+1, d.Name)
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

func printTxDisplay(u ui.UI, d *TxDisplay, network networks.Network) {
	// Transaction summary card.
	statusVal := d.Status
	if d.Status == "done" {
		statusVal = "✓ " + d.Status
	}
	txGroup := [][]ui.TableCell{
		{ui.TC("Status"), ui.TC(statusVal)},
		{ui.TC("From"), tableCell(d.From)},
		{ui.TC("Value"), ui.TC(d.Value + " " + network.GetNativeTokenSymbol())},
		{ui.TC("To"), tableCell(d.To)},
	}
	if d.Hash != "" {
		txGroup = append([][]ui.TableCell{{ui.TC("Hash"), ui.TC(d.Hash)}}, txGroup...)
	}

	if d.Nonce != "" {
		// Degen mode: gas details in the same card, separated by a divider.
		gasGroup := [][]ui.TableCell{
			{ui.TC("Nonce"), ui.TC(d.Nonce)},
			{ui.TC("Gas price"), ui.TC(d.GasPrice + " gwei")},
			{ui.TC("Gas limit"), ui.TC(d.GasLimit)},
			{ui.TC("Gas used"), ui.TC(d.GasUsed)},
			{ui.TC("Gas cost"), ui.TC(d.GasCost)},
		}
		u.PrintTable(&ui.Table{Groups: [][][]ui.TableCell{txGroup, gasGroup}})
	} else {
		u.PrintTable(&ui.Table{Rows: txGroup})
	}

	if d.TxType == "" {
		u.Error("Checking tx type failed: %s", d.Error)
		return
	}
	if d.TxType == "normal" {
		return
	}
	if d.FunctionCall != nil {
		printFunctionCallDisplay(u, d.FunctionCall, false)
	}
	printAllLogs(u, d.Logs)
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
// function call (and any recursively decoded inner calls) and writes it to u.
func DisplayFunctionCall(u ui.UI, fc *jarviscommon.FunctionCall) *FunctionCallDisplay {
	d := buildFunctionCallDisplay(fc, false)
	printFunctionCallDisplay(u, d, false)
	return d
}

// DisplayTxResult builds the human-readable view-model for an analyzed
// transaction and writes it to u. The returned *TxDisplay serializes cleanly
// to JSON (StyledText fields marshal as plain strings); the terminal sees
// coloured output via u.Style.
//
// hash is the transaction hash string shown in the summary card; pass an empty
// string to omit it (e.g. when the hash is already shown by the caller).
func DisplayTxResult(u ui.UI, result *jarviscommon.TxResult, network networks.Network, fullDetail bool, hash string) *TxDisplay {
	d := buildTxDisplay(result, fullDetail)
	d.Hash = hash
	printTxDisplay(u, d, network)
	return d
}
