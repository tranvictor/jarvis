package trezoreum

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"

	"github.com/tranvictor/jarvis/util/account/trezoreum/trezor"
)

type TrezorState int

const (
	Ready                TrezorState = iota // Already unlocked and ready to sign data
	WaitingForPin                           // Expecting PIN in order to unlock the trezor
	WaitingForPassphrase                    // Expecting passphrase in order to unlock the trezor
	Unexpected
)

type Bridge interface {
	// init the connection to trezor via libusb and return the status
	// of the device as well as indication to next step to unlock the
	// device.
	Init() (info *trezor.Features, state TrezorState, err error)

	Unlock() error

	Derive(path accounts.DerivationPath) (common.Address, error)

	Sign(
		path accounts.DerivationPath,
		tx *types.Transaction,
		chainID *big.Int,
	) (common.Address, *types.Transaction, error)

	// SignTypedHash signs an EIP-712 message using precomputed domain
	// separator and struct hashes (Trezor opcode EthereumSignTypedHash).
	// Returns a 65-byte r||s||v signature with v in {27, 28}.
	SignTypedHash(
		path accounts.DerivationPath,
		domainSeparator [32]byte,
		messageHash [32]byte,
	) ([]byte, error)

	// SignTypedData drives the structured EthereumSignTypedData protocol
	// required by Trezor Model T / Safe devices for eth_signTypedData_v4.
	SignTypedData(
		path accounts.DerivationPath,
		td *apitypes.TypedData,
	) ([]byte, error)

	// SignPersonalMessage signs an arbitrary byte string with the
	// Ethereum personal_sign / EIP-191 prefix (Trezor opcode
	// EthereumSignMessage). Returns a 65-byte r||s||v signature with
	// v in {27, 28}.
	SignPersonalMessage(
		path accounts.DerivationPath,
		message []byte,
	) ([]byte, error)

	// Abort sends Cancel without waiting for a reply, so an in-flight
	// sign can be interrupted (Ctrl-C) while blocked on a button wait.
	Abort()

	// Close releases the USB handle. The next Unlock/Init reopens it.
	Close() error

	// ResetAfterFailure best-effort Cancels any leftover workflow and
	// closes the USB handle so the next sign starts from Initialize.
	ResetAfterFailure()
}
