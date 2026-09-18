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
	// ModeFull is every measure, including AI review. Used by --careful and
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
	// HasCode is true when dest has bytecode, including an EIP-7702
	// delegation designator. Empty-code EOAs must not get unverified
	// findings; 7702 dests are checked via their delegated implementation.
	HasCode(addr string) (bool, error)
}

// Completer is the AI (or test) client. The only value it accepts is
// an [ai.Payload] built by [ToPayload].
type Completer interface {
	Complete(ctx context.Context, payload []byte) (ModelReply, error)
}

// ModelReply is the structured JSON the model must return.
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

	// Delegation is the first-hop EIP-7702 target of dest when dest is a
	// delegated EOA. Empty when dest is not 7702. Labels stay off this
	// field; it is hex only.
	Delegation string
	// Authorizations are the type-4 authorization_list entries on the
	// transaction being signed. Empty for every other tx type.
	Authorizations []Authorization

	Book  []BookAddr
	Chain Lookup
	AI    Completer
}

// Authorization is one EIP-7702 SetCodeAuthorization. Authority is the
// recovered signer; empty when recovery fails. Address is the code to
// install (zero means revoke).
type Authorization struct {
	Authority string
	Address   string
	ChainID   uint64
	Nonce     uint64
}

// Report is the ordered list of findings after all selected measures.
type Report struct {
	Findings []Finding
	Skipped  []string
}
