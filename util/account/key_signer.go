package account

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type KeySigner struct {
	chainID int64
	key     *ecdsa.PrivateKey
}

func (self *KeySigner) SignTx(tx *types.Transaction) (*types.Transaction, error) {
	opts, err := bind.NewKeyedTransactorWithChainID(self.key, big.NewInt(self.chainID))
	if err != nil {
		return nil, err
	}
	return opts.Signer(crypto.PubkeyToAddress(self.key.PublicKey), tx)
}

func NewKeySigner(key *ecdsa.PrivateKey, chainID int64) *KeySigner {
	return &KeySigner{chainID, key}
}
