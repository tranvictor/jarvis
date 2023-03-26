package account

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tranvictor/jarvis/util/account/ledgereum"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
	"github.com/tranvictor/jarvis/util/broadcaster"
	"github.com/tranvictor/jarvis/util/reader"
)

func NewTomoAccountFromKeystore(file string, password string) (*Account, error) {
	_, key, err := PrivateKeyFromKeystore(file, password)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewTomoKeySigner(key),
		reader.NewTomoReader(),
		broadcaster.NewTomoBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewTomoAccountFromPrivateKey(hex string) (*Account, error) {
	_, key, err := PrivateKeyFromHex(hex)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewTomoKeySigner(key),
		reader.NewTomoReader(),
		broadcaster.NewTomoBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewTomoAccountFromPrivateKeyFile(file string) (*Account, error) {
	_, key, err := PrivateKeyFromFile(file)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewTomoKeySigner(key),
		reader.NewTomoReader(),
		broadcaster.NewTomoBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewTomoTrezorAccount(path string, address string) (*Account, error) {
	signer, err := trezoreum.NewTrezorTomoSigner(path, address)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader.NewTomoReader(),
		broadcaster.NewTomoBroadcaster(),
		common.HexToAddress(address),
	}, nil
}

func NewTomoLedgerAccount(path string, address string) (*Account, error) {
	signer, err := ledgereum.NewTomoLedgerSigner(path, address)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader.NewTomoReader(),
		broadcaster.NewTomoBroadcaster(),
		common.HexToAddress(address),
	}, nil
}
