package util

import (
	"fmt"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
)

// DisplayParam writes a single decoded ABI parameter to u, including its name,
// type, and value(s). Nested types (tuples, arrays of arrays) are indented via
// u.Indent() rather than a manual integer level.
func DisplayParam(u ui.UI, param jarviscommon.ParamResult) {
	label := fmt.Sprintf("%s (%s)", param.Name, param.Type)
	switch {
	case param.Values != nil:
		if len(param.Values) == 1 {
			u.Info("%s: %s", label, jarviscommon.VerboseValue(param.Values[0]))
		} else {
			u.Info("%s:", label)
			for i, v := range param.Values {
				u.Indent().Info("%d. %s", i+1, jarviscommon.VerboseValue(v))
			}
		}
	case param.Tuples != nil:
		u.Info("%s:", label)
		if len(param.Tuples) == 1 {
			for _, field := range param.Tuples[0].Values {
				DisplayParam(u.Indent(), field)
			}
		} else {
			for i, tuple := range param.Tuples {
				u.Indent().Info("[%d]", i)
				for _, field := range tuple.Values {
					DisplayParam(u.Indent().Indent(), field)
				}
			}
		}
	case param.Arrays != nil:
		u.Info("%s:", label)
		for _, arr := range param.Arrays {
			DisplayParam(u.Indent(), arr)
		}
	}
}

// DisplayFunctionCall writes a decoded function call (and any recursively
// decoded inner calls) to u.
func DisplayFunctionCall(u ui.UI, fc *jarviscommon.FunctionCall) {
	displayFunctionCallInner(u, fc, false)
}

func displayFunctionCallInner(u ui.UI, fc *jarviscommon.FunctionCall, nested bool) {
	if fc.Method == "" {
		u.Error("Getting ABI and function name failed: %s", fc.Error)
		return
	}
	if nested {
		u.Info("Interpreted Contract call to: %s", jarviscommon.VerboseAddress(fc.Destination))
		u.Info("| Value: %f ETH", jarviscommon.BigToFloat(fc.Value, 18))
	} else {
		u.Info("")
	}
	u.Info("| Method: %s", fc.Method)
	u.Info("| Params:")
	for _, param := range fc.Params {
		DisplayParam(u.Indent(), param)
	}
	for _, dfc := range fc.DecodedFunctionCalls {
		displayFunctionCallInner(u.Indent(), dfc, true)
	}
}

// DisplayLog writes a single event log entry (name, indexed topics, data
// params) to u.
func DisplayLog(u ui.UI, idx int, log jarviscommon.LogResult) {
	u.Info("Log %d: %s", idx+1, log.Name)
	inner := u.Indent()
	for j, topic := range log.Topics {
		if len(topic.Value) == 1 {
			inner.Info("Topic %d - %s: %s", j+1, topic.Name, jarviscommon.VerboseValue(topic.Value[0]))
		} else {
			inner.Info("Topic %d - %s:", j+1, topic.Name)
			for k, v := range topic.Value {
				inner.Indent().Info("%d. %s", k+1, jarviscommon.VerboseValue(v))
			}
		}
	}
	inner.Info("Data:")
	for _, param := range log.Data {
		DisplayParam(inner.Indent(), param)
	}
}

// DisplayTxResult writes a transaction result to u.
// When fullDetail is true the gas fields and decoded function call are
// included; when false only the summary (status, from/to, event logs) is shown.
func DisplayTxResult(u ui.UI, result *jarviscommon.TxResult, network networks.Network, fullDetail bool) {
	if result.Status == "done" {
		u.Success("Mining status: %s", result.Status)
	} else {
		u.Error("Mining status: %s", result.Status)
	}

	u.Info("From: %s ===[%s %s]===> %s",
		jarviscommon.VerboseAddress(result.From),
		result.Value, network.GetNativeTokenSymbol(),
		jarviscommon.VerboseAddress(result.To),
	)

	if fullDetail {
		u.Info("Nonce: %s", result.Nonce)
		u.Info("Gas price: %s gwei", result.GasPrice)
		u.Info("Gas limit: %s", result.GasLimit)
	}

	if result.TxType == "" {
		u.Error("Checking tx type failed: %s", result.Error)
		return
	}
	if result.TxType == "normal" {
		return
	}

	if fullDetail && result.FunctionCall != nil {
		DisplayFunctionCall(u, result.FunctionCall)
	}

	u.Info("")
	u.Info("Event logs:")
	for i, l := range result.Logs {
		DisplayLog(u, i, l)
	}
}
