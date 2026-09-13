package vet

import (
	"context"
	"math/big"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

// Mode selects which measures run.
type Mode int

const (
	// ModeAlways is the DELEGATECALL measure only. It runs on every Safe
	// signing card so turning --careful off does not drop that warning.
	ModeAlways Mode = iota
	// ModeFull is every measure, including Grok. Used by --careful and
	// `jarvis vet`.
	ModeFull
)

// Risk is the card colour: caution is yellow, danger is red.
type Risk string

const (
	RiskCaution Risk = "caution"
	RiskDanger  Risk = "danger"
)

// Finding is one line shown on the signing card or `jarvis vet` output.
type Finding struct {
	Code          string
	Risk          Risk
	Text          string
	GrokReconfirm bool
}

// BookAddr is one address-book (or bundled-token) entry used only for
// local measures such as poisoning. Labels stay on the card; they are
// never copied into an AI payload.
type BookAddr struct {
	Hex   string
	Label string
}

// Source is verified Solidity (or flattened multi-file source) from the
// block explorer. Name fields from the explorer are deliberately omitted.
type Source struct {
	Address        string
	Code           string
	Verified       bool
	Implementation string
	IsProxy        bool
}

// Lookup is the chain/explorer surface vet needs. Tests inject fakes;
// production uses [ExplorerLookup].
type Lookup interface {
	Source(addr string) (Source, error)
	Implementation(addr string) (string, error)
}

// Completer is the Grok (or test) client. The only value it accepts is
// an [ai.Payload] built by [ToPayload].
type Completer interface {
	Complete(ctx context.Context, payload []byte) (ModelReply, error)
}

// ModelReply is the structured JSON Grok must return.
type ModelReply struct {
	Risk        string   `json:"risk"`
	Bullets     []string `json:"bullets"`
	AssetEffect string   `json:"asset_effect"`
	Reconfirms  []string `json:"reconfirms"`
}

// Request is everything a measure may inspect. Address-book names live on
// Call.Destination.Desc and Book[i].Label for local display only.
type Request struct {
	Mode        Mode
	ChainID     uint64
	NetworkName string

	To           jarviscommon.Address
	Value        *big.Int
	Data         []byte
	Call         *jarviscommon.FunctionCall
	DelegateCall bool
	MultiSend    bool
	Create       bool

	Book  []BookAddr
	Chain Lookup
	AI    Completer
}

// Report is the ordered list of findings after all selected measures.
type Report struct {
	Findings []Finding
	Skipped  []string
}
