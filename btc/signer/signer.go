// Package signer signs Bitcoin spend plans with a keystore file or a
// hardware wallet. Every signer returns a fully signed transaction; the
// caller runs spend.Plan.Verify on it before anything is broadcast.
package signer

import (
	"context"
	"errors"

	"github.com/btcsuite/btcd/wire"

	"github.com/tranvictor/jarvis/btc/spend"
)

var (
	// ErrUserCancelled means the operator refused on the device or at the
	// password prompt. Nothing was signed.
	ErrUserCancelled = errors.New("signing cancelled")
	// ErrWrongWallet means the key the signer holds is not the one the
	// wallet record describes (wrong passphrase, wrong device, wrong
	// keystore file).
	ErrWrongWallet = errors.New("the signing key does not match the wallet record")
)

type Signer interface {
	Sign(ctx context.Context, p *spend.Plan) (*wire.MsgTx, error)
}

// TxSource fetches raw transactions. Trezor firmware asks for the full
// previous transaction of every input before it will sign.
type TxSource interface {
	TxHex(ctx context.Context, txid string) (string, error)
}
