// Package walletconnect turns jarvis into a WalletConnect v2 wallet so
// browser-hosted dApps (AAVE, KyberSwap, Uniswap, etc.) can route their
// transaction/signature requests to a local jarvis process.
//
// # Model
//
// The command (jarvis wc <uri> --from <account>) is blocking: jarvis
// pairs with the dApp over the provided wc: URI, settles one session,
// and then stays connected — streaming JSON-RPC requests through the
// relay and prompting the user for each one — until the user hits Ctrl-C
// or the dApp disconnects. There is no daemon, no disk-persisted session
// state, no background polling. Everything lives in RAM for the lifetime
// of the process.
//
// # Supported wallets
//
// The --from account can be any wallet jarvis already knows about:
//
//   - EOA (ordinary keystore / Ledger / Trezor): eth_sendTransaction is
//     signed locally and broadcast directly; personal_sign /
//     eth_signTypedData_v4 are signed and returned inline.
//   - Gnosis Safe multisig: eth_sendTransaction is turned into a SafeTx
//     and proposed the same way `jarvis msig init` does (Safe
//     Transaction Service by default, on-chain approval or local
//     tx-file depending on flags). The safeTxHash is returned to the
//     dApp, not an L1 tx hash. Raw signing methods are refused in v1.
//   - Gnosis Classic multisig: eth_sendTransaction is wrapped in
//     submitTransaction and the outer tx hash is returned. Raw signing
//     methods are refused in v1.
//
// Gateway selection happens once at pair-time via cmd/util's existing
// multisig detector, so the same on-chain probe (and disk cache) that
// powers `jarvis msig` also drives which gateway the session uses.
//
// # Package layout
//
//   - uri.go        : wc:<topic>@<version>?... URI parsing & validation
//   - caip.go       : CAIP-2 / CAIP-10 helpers (eip155:<chain>[:addr])
//   - gateway.go    : Gateway interface + shared request/response types
//   - errors.go     : error sentinels shared across gateways / session
//   - classify.go   : jarvis-account -> Gateway factory
//
// Subpackages (to be added in subsequent turns):
//
//   - walletconnect/wccrypto  : x25519 ECDH + HKDF + ChaCha20-Poly1305
//   - walletconnect/relay     : WebSocket transport & irn_* JSON-RPC
//   - walletconnect/session   : pair/propose/settle/request state machine
//   - walletconnect/gateways  : concrete eoa/safe/classic gateways
//
// # Scope boundaries
//
// WC v1 (the 2018-era bridge protocol) is detected and rejected with a
// helpful error — only v2 is implemented. No new top-level Go modules
// are introduced; the transport is built on gorilla/websocket (already
// an indirect dep via go-ethereum) and the crypto primitives on
// golang.org/x/crypto (ditto).
package walletconnect
