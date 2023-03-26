package account

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type TomoKeySigner struct {
	key *ecdsa.PrivateKey
}

func (self *TomoKeySigner) SignTx(tx *types.Transaction) (*types.Transaction, error) {
	opts, err := bind.NewKeyedTransactorWithChainID(self.key, big.NewInt(88))
	if err != nil {
		return nil, err
	}
	return opts.Signer(crypto.PubkeyToAddress(self.key.PublicKey), tx)
}

func NewTomoKeySigner(key *ecdsa.PrivateKey) *TomoKeySigner {
	return &TomoKeySigner{key}
}
