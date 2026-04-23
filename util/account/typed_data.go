package account

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// TypedDataV4 wraps go-ethereum's apitypes.TypedData with JSON parsing
// and helpers to precompute the EIP-712 domain separator + struct hash
// pair required by the hardware-wallet signing interface.
//
// It lives in util/account (not walletconnect) so it can reach into
// Signer implementations without creating an import cycle: Account
// signs via Signer.SignTypedDataHash(domainSep, structHash), and the
// easiest way to produce that pair from a raw JSON dApp payload is to
// bring the parser close to the consumer.
type TypedDataV4 struct {
	apitypes.TypedData
}

// ParseTypedDataV4 parses a raw eth_signTypedData_v4 JSON payload. The
// input is expected to be the already-unwrapped typed-data object (not
// the surrounding JSON-RPC request), i.e. it has top-level "types" /
// "primaryType" / "domain" / "message" keys.
//
// Validation here is minimal: anything apitypes.TypedData accepts we
// accept. Signers enforce device-specific constraints (some Trezor
// firmwares refuse unknown types, etc.) at sign time.
func ParseTypedDataV4(payload []byte) (*TypedDataV4, error) {
	var td apitypes.TypedData
	if err := json.Unmarshal(payload, &td); err != nil {
		return nil, fmt.Errorf("parse eth_signTypedData_v4 payload: %w", err)
	}
	if td.PrimaryType == "" {
		return nil, fmt.Errorf("eth_signTypedData_v4 payload missing primaryType")
	}
	if _, ok := td.Types[td.PrimaryType]; !ok {
		return nil, fmt.Errorf(
			"eth_signTypedData_v4 primaryType %q not defined in types",
			td.PrimaryType)
	}
	if _, ok := td.Types["EIP712Domain"]; !ok {
		return nil, fmt.Errorf("eth_signTypedData_v4 payload missing EIP712Domain type")
	}
	return &TypedDataV4{TypedData: td}, nil
}

// Hashes returns (domainSeparator, structHash). Separated from signing
// so the caller can log / display the digest for operator review before
// committing the hardware wallet to the signature.
func (t *TypedDataV4) Hashes() (domainSep, structHash [32]byte, err error) {
	domainHash, err := t.HashStruct("EIP712Domain", t.Domain.Map())
	if err != nil {
		return domainSep, structHash, fmt.Errorf("hash EIP712Domain: %w", err)
	}
	msgHash, err := t.HashStruct(t.PrimaryType, t.Message)
	if err != nil {
		return domainSep, structHash, fmt.Errorf("hash %s: %w", t.PrimaryType, err)
	}
	if len(domainHash) != 32 || len(msgHash) != 32 {
		return domainSep, structHash, fmt.Errorf(
			"unexpected hash sizes: domain=%d message=%d",
			len(domainHash), len(msgHash))
	}
	copy(domainSep[:], domainHash)
	copy(structHash[:], msgHash)
	return domainSep, structHash, nil
}
