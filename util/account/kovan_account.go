package account

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tranvictor/jarvis/util/account/ledgereum"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
	"github.com/tranvictor/jarvis/util/broadcaster"
	"github.com/tranvictor/jarvis/util/reader"
)

func NewKovanAccountFromKeystore(file string, password string) (*Account, error) {
	_, key, err := PrivateKeyFromKeystore(file, password)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 42),
		reader.NewKovanReader(),
		broadcaster.NewKovanBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewKovanAccountFromPrivateKey(hex string) (*Account, error) {
	_, key, err := PrivateKeyFromHex(hex)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 42),
		reader.NewKovanReader(),
		broadcaster.NewKovanBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewKovanAccountFromPrivateKeyFile(file string) (*Account, error) {
	_, key, err := PrivateKeyFromFile(file)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 42),
		reader.NewKovanReader(),
		broadcaster.NewKovanBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewKovanTrezorAccount(path string, address string) (*Account, error) {
	signer, err := trezoreum.NewTrezorSignerGeneric(path, address, 42)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader.NewKovanReader(),
		broadcaster.NewKovanBroadcaster(),
		common.HexToAddress(address),
	}, nil
}

func NewKovanLedgerAccount(path string, address string) (*Account, error) {
	signer, err := ledgereum.NewLedgerSignerGeneric(path, address, 42)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader.NewKovanReader(),
		broadcaster.NewKovanBroadcaster(),
		common.HexToAddress(address),
	}, nil
}
