package util

import (
	"fmt"

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

// paramSimpleRows returns one [label, value] row for each scalar value in d,
// or nil if d holds a complex type (tuple / array) that needs its own block.
// The returned rows are meant to be batched with rows from sibling params so
// the caller can emit a single unified table rather than one table per param.
func paramSimpleRows(u ui.UI, d ParamDisplay) [][]string {
	label := fmt.Sprintf("%s (%s)", d.Name, d.Type)
	switch {
	case d.Values != nil:
		if len(d.Values) == 1 {
			return [][]string{{label, u.Style(d.Values[0])}}
		}
		rows := make([][]string, len(d.Values))
		for i, v := range d.Values {
			rows[i] = []string{fmt.Sprintf("%s [%d]", label, i+1), u.Style(v)}
		}
		return rows
	}
	return nil
}

// printComplexParam prints tuple / array params that cannot be flattened into
// a simple two-column row. Called after the combined scalar table is emitted.
func printComplexParam(u ui.UI, d ParamDisplay) {
	label := fmt.Sprintf("%s (%s)", d.Name, d.Type)
	switch {
	case d.Tuples != nil:
		u.Info("%s:", label)
		if len(d.Tuples) == 1 {
			printParamList(u.Indent(), d.Tuples[0].Fields)
		} else {
			for i, tuple := range d.Tuples {
				u.Indent().Info("[%d]", i)
				printParamList(u.Indent().Indent(), tuple.Fields)
			}
		}
	case d.Arrays != nil:
		u.Info("%s:", label)
		printParamList(u.Indent(), d.Arrays)
	}
}

// printParamList renders a slice of ParamDisplays:
//   - all scalar (Values) params are collected into one combined table
//   - complex (tuple / array) params are rendered below it
func printParamList(u ui.UI, params []ParamDisplay) {
	var rows [][]string
	for _, p := range params {
		rows = append(rows, paramSimpleRows(u, p)...)
	}
	if len(rows) > 0 {
		u.Table([]string{"Parameter", "Value"}, rows)
	}
	for _, p := range params {
		if p.Tuples != nil || p.Arrays != nil {
			printComplexParam(u, p)
		}
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
		paramGroup = append(paramGroup, paramSimpleRows(u, p)...)
	}

	if len(paramGroup) > 0 {
		u.TableWithGroups(nil, [][][]string{metaGroup, paramGroup})
	} else {
		u.TableWithGroups(nil, [][][]string{metaGroup})
	}

	// Complex params (tuples / arrays) below the table.
	for _, p := range d.Params {
		if p.Tuples != nil || p.Arrays != nil {
			printComplexParam(u, p)
		}
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
		rows = append(rows, paramSimpleRows(u, param)...)
	}
	return rows
}

// printAllLogs renders all event logs as one unified 3-column table
// (Event | Parameter | Value). The event name appears only in the first row of
// each group; subsequent rows in the same log have an empty event cell. Complex
// params (tuples / arrays) that cannot be flattened are printed below the table.
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

	// Complex params (tuples / arrays) are printed per-log below the main table.
	for i, d := range logs {
		for _, param := range d.Data {
			if param.Tuples != nil || param.Arrays != nil {
				u.Info("%d. %s — %s (%s):", i+1, d.Name, param.Name, param.Type)
				printComplexParam(u.Indent(), param)
			}
		}
	}
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
		rows = append(rows, paramSimpleRows(u, param)...)
	}
	if len(rows) > 0 {
		u.Table([]string{"Parameter", "Value"}, rows)
	}
	for _, param := range d.Data {
		if param.Tuples != nil || param.Arrays != nil {
			printComplexParam(u, param)
		}
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
