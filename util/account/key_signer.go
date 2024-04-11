package account

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type KeySigner struct {
	key *ecdsa.PrivateKey
}

func (self *KeySigner) SignTx(tx *types.Transaction, chainId *big.Int) (*types.Transaction, error) {
	opts, err := bind.NewKeyedTransactorWithChainID(self.key, chainId)
	if err != nil {
		return nil, err
	}
	return opts.Signer(crypto.PubkeyToAddress(self.key.PublicKey), tx)
}

func NewKeySigner(key *ecdsa.PrivateKey) *KeySigner {
	return &KeySigner{key}
}
