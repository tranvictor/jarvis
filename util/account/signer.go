package account

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
)

type Signer interface {
	SignTx(tx *types.Transaction, chainId *big.Int) (*types.Transaction, error)
}
