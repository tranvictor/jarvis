package util

// ParamDisplay is the human-readable view-model for a single decoded ABI
// parameter. All string fields contain the verbose form already rendered by
// VerboseValue — what the user sees on screen is identical to what appears in
// JSON.
type ParamDisplay struct {
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	Values []string       `json:"values,omitempty"` // verbose strings for scalar/array-of-scalar params
	Tuples []TupleDisplay `json:"tuples,omitempty"` // one entry per tuple instance
	Arrays []ParamDisplay `json:"arrays,omitempty"` // one entry per inner array
}

// TupleDisplay represents one struct/tuple instance with its decoded fields.
type TupleDisplay struct {
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	Fields []ParamDisplay `json:"fields"`
}

// TopicDisplay is the human-readable view-model for a single indexed event
// argument.
type TopicDisplay struct {
	Name    string `json:"name"`
	Verbose string `json:"verbose"`
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
	Destination string                 `json:"destination"`           // VerboseAddress
	Value       string                 `json:"value,omitempty"`       // ETH value; omitted for top-level
	Method      string                 `json:"method,omitempty"`
	Params      []ParamDisplay         `json:"params,omitempty"`
	InnerCalls  []*FunctionCallDisplay `json:"inner_calls,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

// TxDisplay is the complete human-readable view-model for a single analyzed
// transaction. It contains exactly what was printed to the terminal — nothing
// more, nothing less — so the JSON output and the screen output are guaranteed
// to be identical representations of the same data.
type TxDisplay struct {
	Status string `json:"status"`
	From   string `json:"from"` // VerboseAddress
	To     string `json:"to"`   // VerboseAddress
	Value  string `json:"value"`

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
