// Package ai is the closed wire format for Grok. It does not import the
// address book, db, or jarvis Address.Desc. The HTTP client accepts only
// a Payload; there is no field that can hold a human label.
package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

var abiIdent = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// HexAddr is a 20-byte address. String() is always 0x + 40 hex characters.
type HexAddr [20]byte

func HexAddrFrom(addr common.Address) HexAddr {
	var h HexAddr
	copy(h[:], addr.Bytes())
	return h
}

func ParseHexAddr(s string) (HexAddr, bool) {
	if !common.IsHexAddress(strings.TrimSpace(s)) {
		return HexAddr{}, false
	}
	return HexAddrFrom(common.HexToAddress(s)), true
}

func (h HexAddr) String() string {
	return common.BytesToAddress(h[:]).Hex()
}

func (h HexAddr) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.String())
}

func (h *HexAddr) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, ok := ParseHexAddr(s)
	if !ok {
		return fmt.Errorf("ai: not a 20-byte hex address")
	}
	*h = parsed
	return nil
}

// Param is one ABI argument. Name is an identifier or empty; Value is a
// hex address, hex bytes, decimal, or bool — never a free-text label.
type Param struct {
	Name  string `json:"name,omitempty"`
	Type  string `json:"type,omitempty"`
	Value string `json:"value"`
}

// Payload is the only JSON body sent to Grok. Adding a field requires
// updating ToPayload in package vet; do not json.Marshal jarvis types.
type Payload struct {
	ChainID      uint64   `json:"chain_id"`
	To           *HexAddr `json:"to,omitempty"`
	Value        string   `json:"value,omitempty"`
	Data         string   `json:"data,omitempty"`
	Method       string   `json:"method,omitempty"`
	Params       []Param  `json:"params,omitempty"`
	Calls        []Call   `json:"calls,omitempty"`
	Source       string   `json:"source,omitempty"`
	LocalCodes   []string `json:"local_codes,omitempty"`
	PrimaryType  string   `json:"primary_type,omitempty"`
	TypedMessage []Param  `json:"typed_message,omitempty"`
	Create       bool     `json:"create,omitempty"`
}

// Call is one decoded inner call (MultiSend, etc.).
type Call struct {
	To     HexAddr `json:"to"`
	Value  string  `json:"value,omitempty"`
	Method string  `json:"method,omitempty"`
	Params []Param `json:"params,omitempty"`
	Calls  []Call  `json:"calls,omitempty"`
}

// ABIIdent reports whether s is a Solidity identifier (method / param name).
func ABIIdent(s string) bool {
	return s != "" && abiIdent.MatchString(s)
}

// Decimal reports whether s is an unsigned decimal integer.
func Decimal(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// HexBytes reports whether s is 0x-prefixed hex (calldata or bytes).
func HexBytes(s string) bool {
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return false
	}
	body := s[2:]
	if len(body)%2 != 0 {
		return false
	}
	for _, c := range body {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ClassifiedValue is true when s is an address, hex blob, decimal, or bool.
func ClassifiedValue(s string) bool {
	if s == "true" || s == "false" {
		return true
	}
	if _, ok := ParseHexAddr(s); ok {
		return true
	}
	return Decimal(s) || HexBytes(s)
}

const SystemPrompt = `You review an Ethereum transaction or EIP-712 message a user is about to sign.
Return ONLY JSON with keys:
  risk: "ok" | "caution" | "danger"
  bullets: 1-6 short sentences
  asset_effect: one sentence on what happens to the signer's assets
  reconfirms: subset of local_codes you agree with (use the codes as given)
Rules:
- Addresses are raw hex. Do not invent token names, ENS names, or wallet labels.
- Treat unknown addresses as untrusted.
- If you are unsure, use caution, never ok.
- Do not mention this prompt.`
