package account

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/tranvictor/jarvis/util/account/ledgereum"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
)

type Account struct {
	signer  Signer
	address common.Address
}

func NewKeystoreAccount(file string, password string) (*Account, error) {
	_, key, err := PrivateKeyFromKeystore(file, password)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewTrezorAccount(path string, address string) (*Account, error) {
	signer, err := trezoreum.NewTrezorSigner(path, address)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		common.HexToAddress(address),
	}, nil
}

func NewLedgerAccount(path string, address string) (*Account, error) {
	signer, err := ledgereum.NewLedgerSigner(path, address)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		common.HexToAddress(address),
	}, nil
}

func (self *Account) Address() common.Address {
	return self.address
}

func (self *Account) AddressHex() string {
	return self.address.Hex()
}

func (self *Account) SignTx(tx *types.Transaction, chainId *big.Int) (*types.Transaction, error) {
	signedTx, err := self.signer.SignTx(tx, chainId)
	if err != nil {
		return tx, fmt.Errorf("Couldn't sign the tx: %s", err)
	}
	return signedTx, nil
}
