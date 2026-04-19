package account

import (
	"crypto/ecdsa"
	"fmt"
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

// SignTypedDataHash signs the EIP-712 digest keccak256(0x19 0x01 || ds || mh)
// directly with the wrapped private key and returns r||s||v with v in {27, 28}
// — the canonical EIP-712 form expected by GnosisSafe.checkSignatures.
func (self *KeySigner) SignTypedDataHash(domainSeparator, structHash [32]byte) ([]byte, error) {
	digest := safeEIP712Digest(domainSeparator, structHash)
	sig, err := crypto.Sign(digest[:], self.key)
	if err != nil {
		return nil, fmt.Errorf("crypto.Sign: %w", err)
	}
	if len(sig) != 65 {
		return nil, fmt.Errorf("expected 65-byte signature, got %d", len(sig))
	}
	// crypto.Sign returns v in {0, 1}; Safe expects {27, 28}.
	sig[64] += 27
	return sig, nil
}

func NewKeySigner(key *ecdsa.PrivateKey) *KeySigner {
	return &KeySigner{key}
}

// safeEIP712Digest computes keccak256(0x19 0x01 || domainSeparator || structHash).
// Exposed inside the package so each Signer impl can build the same digest
// when its underlying device only supports raw-hash signing.
func safeEIP712Digest(domainSeparator, structHash [32]byte) [32]byte {
	buf := make([]byte, 0, 2+32+32)
	buf = append(buf, 0x19, 0x01)
	buf = append(buf, domainSeparator[:]...)
	buf = append(buf, structHash[:]...)
	var out [32]byte
	copy(out[:], crypto.Keccak256(buf))
	return out
}
