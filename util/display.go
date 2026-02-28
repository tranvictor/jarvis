package util

import (
	"fmt"
	"strings"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
)

// ── Severity helpers ─────────────────────────────────────────────────────────

// styledAddress wraps a common.Address in a StyledText.
// Known addresses (non-empty, non-"unknown" description) are Success (green);
// unknown ones are Warn (yellow) so they stand out without being alarming.
func styledAddress(addr jarviscommon.Address) ui.StyledText {
	text := jarviscommon.PlainAddress(addr)
	if addr.Desc == "" || addr.Desc == "unknown" {
		return ui.StyledText{Text: text, Severity: ui.SeverityWarn}
	}
	return ui.StyledText{Text: text, Severity: ui.SeveritySuccess}
}

// styledValue wraps a common.Value in a StyledText.
// Address values inherit their severity from styledAddress; all other values
// are SeverityInfo (plain).
func styledValue(v jarviscommon.Value) ui.StyledText {
	if v.Kind == jarviscommon.DisplayAddress && v.Address != nil {
		return styledAddress(*v.Address)
	}
	return ui.StyledText{Text: jarviscommon.PlainValue(v), Severity: ui.SeverityInfo}
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
		Destination: styledAddress(fc.Destination),
		Error:       fc.Error,
		Method:      fc.Method,
	}
	if nested && fc.Value != nil {
		d.Value = fmt.Sprintf("%f ETH", jarviscommon.BigToFloat(fc.Value, 18))
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
		From:   styledAddress(result.From),
		To:     styledAddress(result.To),
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
func flattenParamRows(u ui.UI, d ParamDisplay, indent string) [][]string {
	label := indent + fmt.Sprintf("%s (%s)", d.Name, d.Type)

	// Scalar value(s).
	if d.Values != nil {
		if len(d.Values) == 1 {
			return [][]string{{label, u.Style(d.Values[0])}}
		}
		rows := make([][]string, len(d.Values))
		for i, v := range d.Values {
			rows[i] = []string{fmt.Sprintf("%s [%d]", label, i+1), u.Style(v)}
		}
		return rows
	}

	// Single tuple: header row + indented children.
	if d.Tuples != nil && len(d.Tuples) == 1 {
		rows := [][]string{{label, ""}}
		for _, field := range d.Tuples[0].Fields {
			rows = append(rows, flattenParamRows(u, field, indent+"  ")...)
		}
		return rows
	}

	// Multi-tuple (array of structs): header row + indexed children.
	if d.Tuples != nil {
		rows := [][]string{{label, ""}}
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
				rows = append(rows, flattenParamRows(u, field, prefix)...)
			}
		}
		return rows
	}

	// Plain array.
	if d.Arrays != nil {
		rows := [][]string{{label, ""}}
		for _, elem := range d.Arrays {
			rows = append(rows, flattenParamRows(u, elem, indent+"  ")...)
		}
		return rows
	}

	return nil
}

// printParamList renders a slice of ParamDisplays as a single unified
// TableWithGroups. Consecutive scalar params share a group; each complex
// param (tuple / array) gets its own group.
func printParamList(u ui.UI, params []ParamDisplay) {
	var groups [][][]string
	var scalarGroup [][]string

	flushScalars := func() {
		if len(scalarGroup) > 0 {
			groups = append(groups, scalarGroup)
			scalarGroup = nil
		}
	}

	for _, p := range params {
		rows := flattenParamRows(u, p, "")
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
		u.TableWithGroups([]string{"Parameter", "Value"}, groups)
	}
}

func printFunctionCallDisplay(u ui.UI, d *FunctionCallDisplay, nested bool) {
	if d.Method == "" {
		u.Error("Getting ABI and function name failed: %s", d.Error)
		return
	}

	if nested {
		// Inner calls are visually subordinate — a simple arrow label, no Section.
		u.Info("↳ %s  [%s]", d.Method, u.Style(d.Destination))
		printParamList(u.Indent(), d.Params)
		for _, inner := range d.InnerCalls {
			printFunctionCallDisplay(u.Indent(), inner, true)
		}
		return
	}

	u.Section(fmt.Sprintf("Function call: %s", d.Method))

	// Build a single TableWithGroups: contract metadata (group 0) + params (group 1).
	metaGroup := [][]string{{"Contract", u.Style(d.Destination)}}
	if d.Value != "" {
		metaGroup = append(metaGroup, []string{"Value", d.Value})
	}

	var paramGroup [][]string
	for _, p := range d.Params {
		paramGroup = append(paramGroup, flattenParamRows(u, p, "")...)
	}

	if len(paramGroup) > 0 {
		u.TableWithGroups(nil, [][][]string{metaGroup, paramGroup})
	} else {
		u.TableWithGroups(nil, [][][]string{metaGroup})
	}

	for _, inner := range d.InnerCalls {
		printFunctionCallDisplay(u.Indent(), inner, true)
	}
}

// logSimpleRows returns all simple [param, value] rows for a single log,
// used when building the combined all-logs table.
func logSimpleRows(u ui.UI, d LogDisplay) [][]string {
	var rows [][]string
	for _, topic := range d.Topics {
		rows = append(rows, []string{topic.Name + " (indexed)", u.Style(topic.Verbose)})
	}
	for _, param := range d.Data {
		rows = append(rows, flattenParamRows(u, param, "")...)
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

	groups := make([][][]string, len(logs))
	for i, d := range logs {
		eventLabel := fmt.Sprintf("%d. %s", i+1, d.Name)
		paramRows := logSimpleRows(u, d)
		if len(paramRows) == 0 {
			groups[i] = [][]string{{eventLabel, "", ""}}
			continue
		}
		group := make([][]string, len(paramRows))
		for j, pr := range paramRows {
			name := ""
			if j == 0 {
				name = eventLabel
			}
			group[j] = []string{name, pr[0], pr[1]}
		}
		groups[i] = group
	}
	u.TableWithGroups([]string{"Event", "Parameter", "Value"}, groups)
}

// printLogDisplay renders a single log entry. Used by the standalone DisplayLog
// public API when a caller needs to print one log outside of a full tx context.
func printLogDisplay(u ui.UI, idx int, d LogDisplay) {
	u.Section(fmt.Sprintf("Log %d: %s", idx+1, d.Name))

	var rows [][]string
	for _, topic := range d.Topics {
		rows = append(rows, []string{topic.Name + " (indexed)", u.Style(topic.Verbose)})
	}
	for _, param := range d.Data {
		rows = append(rows, flattenParamRows(u, param, "")...)
	}
	if len(rows) > 0 {
		u.Table([]string{"Parameter", "Value"}, rows)
	}
}

func printTxDisplay(u ui.UI, d *TxDisplay, network networks.Network) {
	// Transaction summary card.
	statusVal := d.Status
	if d.Status == "done" {
		statusVal = "✓ " + d.Status
	}
	txGroup := [][]string{
		{"Status", statusVal},
		{"From", u.Style(d.From)},
		{"Value", d.Value + " " + network.GetNativeTokenSymbol()},
		{"To", u.Style(d.To)},
	}
	if d.Hash != "" {
		txGroup = append([][]string{{"Hash", d.Hash}}, txGroup...)
	}

	if d.Nonce != "" {
		// Degen mode: gas details in the same card, separated by a divider.
		gasGroup := [][]string{
			{"Nonce", d.Nonce},
			{"Gas price", d.GasPrice + " gwei"},
			{"Gas limit", d.GasLimit},
			{"Gas used", d.GasUsed},
			{"Gas cost", d.GasCost},
		}
		u.TableWithGroups(nil, [][][]string{txGroup, gasGroup})
	} else {
		u.Table(nil, txGroup)
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

// DisplayLog builds the human-readable view-model for a single event log entry
// and writes it to u.
func DisplayLog(u ui.UI, idx int, log jarviscommon.LogResult) LogDisplay {
	d := buildLogDisplay(log)
	printLogDisplay(u, idx, d)
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
