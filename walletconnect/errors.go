package walletconnect

import "fmt"

// Error sentinels shared across gateways and the session loop. Gateway
// implementations return these (or wrap them) to let the session layer
// translate into correct WC JSON-RPC error codes without having to
// introspect error strings.
//
// The WC v2 spec pins a handful of error codes
// (https://specs.walletconnect.com/2.0/specs/clients/sign/error-codes);
// mapping is done by the session layer based on which sentinel we unwrap.
var (
	// ErrMethodNotSupported is returned by a Gateway when the dApp
	// asked for a JSON-RPC method the underlying wallet can't service
	// at all (e.g. personal_sign on a Gnosis Safe — a multisig cannot
	// produce an EOA-style signature). Maps to WC error 5101 /
	// "Unsupported method".
	ErrMethodNotSupported = fmt.Errorf("walletconnect: method not supported by this wallet")

	// ErrChainNotSupported is returned by a Gateway's SwitchChain (or
	// any other method that receives a chain argument) when the target
	// chain isn't usable for this wallet: no node config for an EOA,
	// no Safe contract on the target chain for a Safe gateway, or no
	// classic-msig code at the target address for a Classic gateway.
	// Maps to WC error 5100 / "Unsupported chain".
	ErrChainNotSupported = fmt.Errorf("walletconnect: chain not supported by this wallet")

	// ErrUserRejected is returned when the human operator said no at
	// the terminal prompt. Maps to WC error 5000 / "User rejected".
	ErrUserRejected = fmt.Errorf("walletconnect: rejected by user")

	// ErrSessionExpired is returned by the session layer when a
	// pairing or session has exceeded its advertised expiry and can
	// no longer be used. The command loop translates this into an
	// exit with a clear message so the user can re-pair.
	ErrSessionExpired = fmt.Errorf("walletconnect: session expired")
)
