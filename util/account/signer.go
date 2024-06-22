package account

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type Signer interface {
	SignTx(tx *types.Transaction, chainId *big.Int) (common.Address, *types.Transaction, error)
}
