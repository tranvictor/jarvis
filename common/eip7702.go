package common

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

// BuildSetCodeTx builds an EIP-7702 type-4 transaction. to is required
// (type 4 cannot create a contract). auths must already be signed.
func BuildSetCodeTx(
	nonce uint64,
	to string,
	ethAmount *big.Int,
	gasLimit uint64,
	priceGwei float64,
	tipCapGwei float64,
	data []byte,
	chainID uint64,
	auths []types.SetCodeAuthorization,
) *types.Transaction {
	toAddress := common.HexToAddress(to)
	return types.NewTx(&types.SetCodeTx{
		ChainID:   u256FromBig(big.NewInt(int64(chainID))),
		Nonce:     nonce,
		GasTipCap: u256FromBig(GweiToWei(tipCapGwei)),
		GasFeeCap: u256FromBig(GweiToWei(priceGwei)),
		Gas:       gasLimit,
		To:        toAddress,
		Value:     u256FromBig(ethAmount),
		Data:      data,
		AuthList:  auths,
	})
}

func u256FromBig(n *big.Int) *uint256.Int {
	if n == nil {
		return uint256.NewInt(0)
	}
	v, overflow := uint256.FromBig(n)
	if overflow {
		return uint256.NewInt(0)
	}
	return v
}

// TxAuthorizationsFrom copies a type-4 authorization_list. resolve, when
// non-nil, turns hex addresses into labelled Address values.
func TxAuthorizationsFrom(tx *types.Transaction, resolve func(string) Address) []TxAuthorization {
	if tx == nil {
		return nil
	}
	auths := tx.SetCodeAuthorizations()
	if len(auths) == 0 {
		return nil
	}
	if resolve == nil {
		resolve = func(s string) Address { return Address{Address: s} }
	}
	out := make([]TxAuthorization, 0, len(auths))
	for _, a := range auths {
		item := TxAuthorization{
			Address: resolve(a.Address.Hex()),
			ChainID: fmt.Sprintf("%d", a.ChainID.Uint64()),
			Nonce:   fmt.Sprintf("%d", a.Nonce),
			Revoke:  a.Address == (common.Address{}),
		}
		if auth, err := a.Authority(); err == nil {
			item.Authority = resolve(auth.Hex())
		}
		out = append(out, item)
	}
	return out
}
