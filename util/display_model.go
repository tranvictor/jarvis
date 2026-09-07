package util

import "github.com/tranvictor/jarvis/ui"

// ParamDisplay is the human-readable view-model for a single decoded ABI
// parameter. Each value is a StyledText — the plain text serializes cleanly
// to JSON while the Severity annotation drives terminal coloring via u.Style.
type ParamDisplay struct {
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Values []ui.StyledText `json:"values,omitempty"` // serializes as []string
	Tuples []TupleDisplay  `json:"tuples,omitempty"`
	Arrays []ParamDisplay  `json:"arrays,omitempty"`
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
	Name    string        `json:"name"`
	Verbose ui.StyledText `json:"verbose"` // serializes as string
}

// LogDisplay is the human-readable view-model for a single event log entry.
type LogDisplay struct {
	Name    string         `json:"name"`
	Address ui.StyledText  `json:"address,omitempty"` // emitting contract; serializes as string
	Topics  []TopicDisplay `json:"topics"`
	Data    []ParamDisplay `json:"data"`
}

// FunctionCallDisplay is the human-readable view-model for a decoded function
// call, including any recursively decoded inner calls (e.g. Gnosis multisig
// submitTransaction wrapping an inner ERC20 transfer).
type FunctionCallDisplay struct {
	Destination ui.StyledText  `json:"destination"` // serializes as string
	Value       string         `json:"value,omitempty"`
	Method      string         `json:"method,omitempty"`
	Params      []ParamDisplay `json:"params,omitempty"`
	// Data is the raw calldata, hex-encoded. It is only populated when the call
	// couldn't be decoded, so the operator can see which contract and payload
	// the failure refers to.
	Data       string                 `json:"data,omitempty"`
	InnerCalls []*FunctionCallDisplay `json:"inner_calls,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

// TxLayout selects how much of a TxDisplay is rendered and how densely.
// The view-model is always built in full; the layout only affects printing.
type TxLayout int

const (
	// LayoutInfo is the default for `jarvis info`: headline, transfers,
	// collapsed call, one line per event, shortened addresses.
	LayoutInfo TxLayout = iota
	// LayoutInfoFull is `jarvis info -x`: nothing collapsed, gas card, full
	// addresses, events as a table.
	LayoutInfoFull
	// LayoutPostSign is shown right after a tx the user just signed was
	// mined: the call was already confirmed on the signing screen, so only
	// status, gas, transfers and events are printed (the call reappears only
	// when the tx reverted).
	LayoutPostSign
)

// TransferDisplay is one asset movement derived from the event logs
// (ERC-20/721 Transfer, Approval, WETH Deposit/Withdrawal).
type TransferDisplay struct {
	Kind   string        `json:"kind"` // transfer | approval | deposit | withdrawal
	Token  ui.StyledText `json:"token"`
	Amount string        `json:"amount"`
	From   ui.StyledText `json:"from,omitempty"`
	To     ui.StyledText `json:"to,omitempty"`
	// Unlimited marks a max-uint approval, which deserves attention.
	Unlimited bool `json:"unlimited,omitempty"`
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

	Nonce       string `json:"nonce,omitempty"`
	GasPrice    string `json:"gas_price,omitempty"`
	GasLimit    string `json:"gas_limit,omitempty"`
	GasUsed     string `json:"gas_used,omitempty"`
	GasCost     string `json:"gas_cost,omitempty"`
	BlockNumber string `json:"block_number,omitempty"`

	TxType       string               `json:"tx_type"`
	FunctionCall *FunctionCallDisplay `json:"function_call,omitempty"`
	Transfers    []TransferDisplay    `json:"transfers,omitempty"`
	Logs         []LogDisplay         `json:"logs,omitempty"`
	// RevertReason is the decoded revert payload of a reverted tx, when the
	// replay could recover one.
	RevertReason string `json:"revert_reason,omitempty"`
	Error        string `json:"error,omitempty"`
}
