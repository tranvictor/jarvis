package account

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type KeySigner struct {
	key *ecdsa.PrivateKey
}

func (self *KeySigner) SignTx(
	tx *types.Transaction,
	chainId *big.Int,
) (common.Address, *types.Transaction, error) {
	opts, err := bind.NewKeyedTransactorWithChainID(self.key, chainId)
	if err != nil {
		return common.Address{}, nil, err
	}
	addr := crypto.PubkeyToAddress(self.key.PublicKey)
	signedTx, err := opts.Signer(addr, tx)
	return addr, signedTx, err
}

func NewKeySigner(key *ecdsa.PrivateKey) *KeySigner {
	return &KeySigner{key}
}
