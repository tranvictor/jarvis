package trezoreum

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/usbwallet/trezor"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
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
	Init() (info trezor.Features, state TrezorState, err error)

	Unlock() error

	Derive(path accounts.DerivationPath) (common.Address, error)

	Sign(path accounts.DerivationPath, tx *types.Transaction, chainID *big.Int) (common.Address, *types.Transaction, error)
}
