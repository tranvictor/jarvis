package util

import "github.com/tranvictor/jarvis/ui"

// ParamDisplay is the human-readable view-model for a single decoded ABI
// parameter. Each value is a StyledText — the plain text serializes cleanly
// to JSON while the Severity annotation drives terminal coloring via u.Style.
type ParamDisplay struct {
	Name   string           `json:"name"`
	Type   string           `json:"type"`
	Values []ui.StyledText  `json:"values,omitempty"` // serializes as []string
	Tuples []TupleDisplay   `json:"tuples,omitempty"`
	Arrays []ParamDisplay   `json:"arrays,omitempty"`
}

// TupleDisplay represents one struct/tuple instance with its decoded fields.
type TupleDisplay struct {
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	Fields []ParamDisplay `json:"fields"`
}

// TopicDisplay is the human-readable view-model for a single indexed event
// argument. Verbose is a StyledText so addresses can be rendered in colour
// on the terminal while JSON receives only clean text.
type TopicDisplay struct {
	Name    string         `json:"name"`
	Verbose ui.StyledText  `json:"verbose"` // serializes as string
}

// LogDisplay is the human-readable view-model for a single event log entry.
type LogDisplay struct {
	Name   string         `json:"name"`
	Topics []TopicDisplay `json:"topics"`
	Data   []ParamDisplay `json:"data"`
}

// FunctionCallDisplay is the human-readable view-model for a decoded function
// call, including any recursively decoded inner calls (e.g. Gnosis multisig
// submitTransaction wrapping an inner ERC20 transfer).
type FunctionCallDisplay struct {
	Destination ui.StyledText          `json:"destination"` // serializes as string
	Value       string                 `json:"value,omitempty"`
	Method      string                 `json:"method,omitempty"`
	Params      []ParamDisplay         `json:"params,omitempty"`
	InnerCalls  []*FunctionCallDisplay `json:"inner_calls,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

// TxDisplay is the complete human-readable view-model for a single analyzed
// transaction. StyledText fields carry Severity annotations used only by the
// terminal print phase; JSON consumers receive clean plain strings.
type TxDisplay struct {
	Hash   string        `json:"hash,omitempty"`
	Status string        `json:"status"`
	From   ui.StyledText `json:"from"` // serializes as string
	To     ui.StyledText `json:"to"`   // serializes as string
	Value  string        `json:"value"`

	// Gas/nonce detail — populated only when fullDetail (degen) mode is on.
	Nonce    string `json:"nonce,omitempty"`
	GasPrice string `json:"gas_price,omitempty"`
	GasLimit string `json:"gas_limit,omitempty"`
	GasUsed  string `json:"gas_used,omitempty"`
	GasCost  string `json:"gas_cost,omitempty"`

	TxType       string               `json:"tx_type"`
	FunctionCall *FunctionCallDisplay `json:"function_call,omitempty"`
	Logs         []LogDisplay         `json:"logs,omitempty"`
	Error        string               `json:"error,omitempty"`
}
