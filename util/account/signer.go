package account

import (
	"github.com/ethereum/go-ethereum/core/types"
)

type Signer interface {
	SignTx(tx *types.Transaction) (*types.Transaction, error)
}
