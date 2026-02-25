package util

import (
	"fmt"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
)

// ── Build phase (pure: no UI side-effects) ──────────────────────────────────

func buildParamDisplay(param jarviscommon.ParamResult) ParamDisplay {
	d := ParamDisplay{Name: param.Name, Type: param.Type}
	switch {
	case param.Values != nil:
		for _, v := range param.Values {
			d.Values = append(d.Values, jarviscommon.VerboseValue(v))
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
		Destination: jarviscommon.VerboseAddress(fc.Destination),
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
			Verbose: jarviscommon.VerboseValue(topic.Value),
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
		From:   jarviscommon.VerboseAddress(result.From),
		To:     jarviscommon.VerboseAddress(result.To),
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

// ── Print phase (reads only from the display struct) ────────────────────────

func printParamDisplay(u ui.UI, d ParamDisplay) {
	label := fmt.Sprintf("%s (%s)", d.Name, d.Type)
	switch {
	case d.Values != nil:
		if len(d.Values) == 1 {
			u.Info("%s: %s", label, d.Values[0])
		} else {
			u.Info("%s:", label)
			for i, v := range d.Values {
				u.Indent().Info("%d. %s", i+1, v)
			}
		}
	case d.Tuples != nil:
		u.Info("%s:", label)
		if len(d.Tuples) == 1 {
			for _, field := range d.Tuples[0].Fields {
				printParamDisplay(u.Indent(), field)
			}
		} else {
			for i, tuple := range d.Tuples {
				u.Indent().Info("[%d]", i)
				for _, field := range tuple.Fields {
					printParamDisplay(u.Indent().Indent(), field)
				}
			}
		}
	case d.Arrays != nil:
		u.Info("%s:", label)
		for _, arr := range d.Arrays {
			printParamDisplay(u.Indent(), arr)
		}
	}
}

func printFunctionCallDisplay(u ui.UI, d *FunctionCallDisplay, nested bool) {
	if d.Method == "" {
		u.Error("Getting ABI and function name failed: %s", d.Error)
		return
	}
	if nested {
		u.Info("Interpreted Contract call to: %s", d.Destination)
		u.Info("| Value: %s", d.Value)
	} else {
		u.Info("")
	}
	u.Info("| Method: %s", d.Method)
	u.Info("| Params:")
	for _, param := range d.Params {
		printParamDisplay(u.Indent(), param)
	}
	for _, inner := range d.InnerCalls {
		printFunctionCallDisplay(u.Indent(), inner, true)
	}
}

func printLogDisplay(u ui.UI, idx int, d LogDisplay) {
	u.Info("Log %d: %s", idx+1, d.Name)
	inner := u.Indent()
	for j, topic := range d.Topics {
		inner.Info("Topic %d - %s: %s", j+1, topic.Name, topic.Verbose)
	}
	inner.Info("Data:")
	for _, param := range d.Data {
		printParamDisplay(inner.Indent(), param)
	}
}

func printTxDisplay(u ui.UI, d *TxDisplay, network networks.Network) {
	if d.Status == "done" {
		u.Success("Mining status: %s", d.Status)
	} else {
		u.Error("Mining status: %s", d.Status)
	}
	u.Info("From: %s ===[%s %s]===> %s",
		d.From, d.Value, network.GetNativeTokenSymbol(), d.To,
	)
	if d.Nonce != "" {
		u.Info("Nonce: %s", d.Nonce)
		u.Info("Gas price: %s gwei", d.GasPrice)
		u.Info("Gas limit: %s", d.GasLimit)
		u.Info("Gas used: %s", d.GasUsed)
		u.Info("Gas cost: %s", d.GasCost)
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
	u.Info("")
	u.Info("Event logs:")
	for i, l := range d.Logs {
		printLogDisplay(u, i, l)
	}
}

// ── Public API ───────────────────────────────────────────────────────────────

// DisplayParam builds the human-readable view-model for a single decoded ABI
// parameter and writes it to u. The returned ParamDisplay is the authoritative
// data — the UI output is derived entirely from it.
func DisplayParam(u ui.UI, param jarviscommon.ParamResult) ParamDisplay {
	d := buildParamDisplay(param)
	printParamDisplay(u, d)
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
// transaction and writes it to u. The returned *TxDisplay is the
// authoritative source for JSON output — the terminal output is derived
// entirely from it, guaranteeing both representations are always in sync.
func DisplayTxResult(u ui.UI, result *jarviscommon.TxResult, network networks.Network, fullDetail bool) *TxDisplay {
	d := buildTxDisplay(result, fullDetail)
	printTxDisplay(u, d, network)
	return d
}
