package account

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tranvictor/jarvis/util/account/ledgereum"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
	"github.com/tranvictor/jarvis/util/broadcaster"
	"github.com/tranvictor/jarvis/util/reader"
)

func NewAccountFromKeystore(file string, password string) (*Account, error) {
	_, key, err := PrivateKeyFromKeystore(file, password)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 1),
		reader.NewEthReader(),
		broadcaster.NewBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewAccountFromPrivateKey(hex string) (*Account, error) {
	_, key, err := PrivateKeyFromHex(hex)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 1),
		reader.NewEthReader(),
		broadcaster.NewBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewAccountFromPrivateKeyFile(file string) (*Account, error) {
	_, key, err := PrivateKeyFromFile(file)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 1),
		reader.NewEthReader(),
		broadcaster.NewBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewLedgerAccount(path string, address string) (*Account, error) {
	signer, err := ledgereum.NewLedgerSignerGeneric(path, address, 1)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		// nil,
		reader.NewEthReader(),
		broadcaster.NewBroadcaster(),
		common.HexToAddress(address),
	}, nil
}

func NewTrezorAccount(path string, address string) (*Account, error) {
	signer, err := trezoreum.NewTrezorSignerGeneric(path, address, 1)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader.NewEthReader(),
		broadcaster.NewBroadcaster(),
		common.HexToAddress(address),
	}, nil
}
