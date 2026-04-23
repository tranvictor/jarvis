package account

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Signer is the minimal capability set expected of every wallet backend
// (private key, keystore, Ledger, Trezor, ...).
type Signer interface {
	// SignTx signs an Ethereum transaction.
	SignTx(tx *types.Transaction, chainId *big.Int) (common.Address, *types.Transaction, error)

	// SignTypedDataHash signs an EIP-712 digest given its precomputed
	// domain separator and struct (message) hash.
	//
	// The 65-byte signature returned MUST be a Safe-compatible
	// signature, i.e. one of:
	//   - canonical EIP-712: r || s || v with v in {27, 28} (the
	//     Safe contract treats this as a normal EIP-712 signature).
	//   - eth_sign-compatible: r || s || (v + 4) with v in {31, 32}
	//     (the Safe contract subtracts 4 and re-hashes the input as a
	//     personal_sign message before recovery).
	//
	// Implementations may pick whichever variant their backend natively
	// supports; both are accepted by Safe v1.1.x .. v1.4.x.
	SignTypedDataHash(domainSeparator, structHash [32]byte) ([]byte, error)

	// SignPersonalMessage signs an arbitrary message with the EIP-191
	// "personal_sign" prefix, i.e. the device (or key) hashes and
	// signs keccak256("\x19Ethereum Signed Message:\n" || len(msg) || msg).
	//
	// The returned signature is the canonical 65-byte r||s||v form with
	// v in {27, 28} — the shape WalletConnect dApps, viem/ethers, and
	// EIP-191-verifying contracts all expect. Hardware wallets that
	// natively return v in {0, 1} MUST normalise before returning.
	//
	// This is intentionally separate from SignTypedDataHash because
	// eth_sign (raw keccak over the message) is a footgun we do not
	// want to expose — a hostile dApp could trick a user into signing
	// an unprefixed 32-byte value that is also a valid tx hash.
	SignPersonalMessage(message []byte) ([]byte, error)
}
