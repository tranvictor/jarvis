package account

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tranvictor/jarvis/util/account/ledgereum"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
	"github.com/tranvictor/jarvis/util/broadcaster"
	"github.com/tranvictor/jarvis/util/reader"
)

func NewBSCAccountFromKeystore(file string, password string) (*Account, error) {
	_, key, err := PrivateKeyFromKeystore(file, password)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 56),
		reader.NewBSCReader(),
		broadcaster.NewBSCBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewBSCAccountFromPrivateKey(hex string) (*Account, error) {
	_, key, err := PrivateKeyFromHex(hex)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 56),
		reader.NewBSCReader(),
		broadcaster.NewBSCBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewBSCAccountFromPrivateKeyFile(file string) (*Account, error) {
	_, key, err := PrivateKeyFromFile(file)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 56),
		reader.NewBSCReader(),
		broadcaster.NewBSCBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewBSCTrezorAccount(path string, address string) (*Account, error) {
	signer, err := trezoreum.NewTrezorSignerGeneric(path, address, 56)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader.NewBSCReader(),
		broadcaster.NewBSCBroadcaster(),
		common.HexToAddress(address),
	}, nil
}

func NewBSCLedgerAccount(path string, address string) (*Account, error) {
	signer, err := ledgereum.NewLedgerSignerGeneric(path, address, 56)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader.NewBSCReader(),
		broadcaster.NewBSCBroadcaster(),
		common.HexToAddress(address),
	}, nil
}
