package walletconnect

import (
	"context"
	"math/big"
)

// Gateway is the wallet-shaped adapter the session loop talks to when
// it unwraps a JSON-RPC request from the dApp. There is exactly one
// gateway per session, chosen at pair time by classify.go based on the
// --from account's on-chain shape.
//
// All methods take a context scoped to the single request so cancelling
// the session (Ctrl-C, relay disconnect) also cancels the prompt the
// user might be staring at. Implementations must not retain the ctx
// past return.
//
// Error handling contract:
//
//   - Return ErrUserRejected when the human declined the prompt.
//   - Return ErrMethodNotSupported when the method makes no sense for
//     this wallet kind (e.g. personal_sign on a Safe).
//   - Return ErrChainNotSupported when the requested chain isn't
//     usable (no node config, multisig missing on target chain, ...).
//   - Return any other error to surface it to the dApp as a generic
//     "internal error" — include enough detail in the message so the
//     user reading jarvis's console can tell what failed.
//
// The session layer translates these sentinels into the right
// WalletConnect error codes; gateways should not hand-roll codes
// themselves.
type Gateway interface {
	// Kind returns a short identifier ("eoa" / "safe" / "classic")
	// used in user-facing prompts and session announcements. This is
	// informational only — the session layer does not switch on Kind.
	Kind() string

	// Account returns the CAIP-10 account string this gateway
	// operates on, e.g. "eip155:1:0xabc...". Stable for the lifetime
	// of the gateway instance.
	Account() string

	// Chains returns the CAIP-2 chain IDs this gateway is willing to
	// advertise during session proposal negotiation. The result must
	// include the gateway's primary chain and may include others that
	// wallet_switchEthereumChain can move to without re-pairing.
	//
	// Examples:
	//   - EOA: every chain jarvis has a node config for.
	//   - Safe: every chain where the same Safe address has code and
	//     exposes the Safe ABI (detected lazily).
	//   - Classic: every chain where the same address has code
	//     matching a classic multisig.
	Chains(ctx context.Context) ([]string, error)

	// Methods returns the JSON-RPC method names the session should
	// advertise as supported. Unsupported requests are short-circuited
	// by the session layer before reaching the gateway.
	Methods() []string

	// SendTransaction handles eth_sendTransaction. The returned
	// string is echoed back to the dApp as the "transaction hash" —
	// which for multisigs is a lie-by-design (Safe returns safeTxHash,
	// Classic returns the outer tx hash that submitted the inner
	// one). dApps that treat this as a canonical L1 hash will
	// eventually notice and either re-sync on their own or handle the
	// discrepancy themselves; that mismatch is inherent to using a
	// multisig as a WC signer and is consistent with how Safe{Wallet}
	// behaves.
	SendTransaction(ctx context.Context, chain string, tx *RawTx) (hash string, err error)

	// PersonalSign handles personal_sign(message, address).
	// Implementations that cannot produce an EOA-style signature
	// (Safe, Classic) should return ErrMethodNotSupported.
	PersonalSign(ctx context.Context, chain string, message []byte) (signature string, err error)

	// SignTypedData handles eth_signTypedData_v4. typedDataJSON is
	// the raw JSON payload the dApp sent; gateways parse it
	// themselves because EIP-712 field typing is lossy to go through
	// map[string]interface{}. Implementations that cannot produce an
	// EOA-style signature should return ErrMethodNotSupported.
	SignTypedData(ctx context.Context, chain string, typedDataJSON []byte) (signature string, err error)

	// SwitchChain handles wallet_switchEthereumChain. The gateway
	// should validate the requested chain against whatever the
	// underlying wallet requires and return ErrChainNotSupported on
	// mismatch. On success the gateway updates its own primary-chain
	// state and subsequent SendTransaction / Sign* calls without a
	// chain argument default to the new chain.
	SwitchChain(ctx context.Context, chain string) error
}

// RawTx is the already-normalised transaction payload a dApp included
// in eth_sendTransaction. The session layer parses the WC request JSON
// into this struct before calling the gateway so every gateway sees
// the same shape.
//
// Fields mirror the EIP-1559 / legacy tx common ground:
//
//   - From is validated against the gateway's own account at the session
//     layer; gateways may assume From is lowercased-hex and non-empty.
//   - To is empty for contract creation. Lowercased-hex otherwise.
//   - Value defaults to zero when absent from the request.
//   - Data defaults to empty.
//   - Gas / fee fields are 0 when the dApp omitted them — gateways that
//     need them must estimate themselves.
//   - Nonce is 0 when not supplied (dApps very rarely supply one; the
//     EOA gateway fetches the pending nonce from the node).
type RawTx struct {
	From                 string
	To                   string
	Value                *big.Int
	Data                 []byte
	Gas                  uint64
	GasPrice             *big.Int
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
	Nonce                uint64
	// NonceProvided is set to true by the parser iff the dApp
	// explicitly supplied a nonce — distinguishes "nonce=0 because
	// unset" from "nonce=0 intentionally".
	NonceProvided bool
}

// SupportedMethods is the canonical set of JSON-RPC method strings
// jarvis recognises in session proposals. Individual gateways narrow
// this based on capability. Having a shared constant list prevents
// typos from drifting between gateway.Methods() implementations and
// the session-layer dispatcher.
var SupportedMethods = struct {
	SendTransaction     string
	PersonalSign        string
	SignTypedDataV4     string
	SwitchChain         string
	AddChain            string
	// Read-only methods we forward without prompting. Added here so
	// the session layer has a single source of truth for the
	// distinction, even though individual gateways also list them.
	ChainID       string
	Accounts      string
	BlockNumber   string
}{
	SendTransaction: "eth_sendTransaction",
	PersonalSign:    "personal_sign",
	SignTypedDataV4: "eth_signTypedData_v4",
	SwitchChain:     "wallet_switchEthereumChain",
	AddChain:        "wallet_addEthereumChain",
	ChainID:         "eth_chainId",
	Accounts:        "eth_accounts",
	BlockNumber:     "eth_blockNumber",
}
