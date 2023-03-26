package account

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tranvictor/jarvis/util/account/ledgereum"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
	"github.com/tranvictor/jarvis/util/broadcaster"
	"github.com/tranvictor/jarvis/util/reader"
)

func NewRopstenAccountFromKeystore(file string, password string) (*Account, error) {
	_, key, err := PrivateKeyFromKeystore(file, password)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 3),
		reader.NewRopstenReader(),
		broadcaster.NewRopstenBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewRopstenAccountFromPrivateKey(hex string) (*Account, error) {
	_, key, err := PrivateKeyFromHex(hex)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 3),
		reader.NewRopstenReader(),
		broadcaster.NewRopstenBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewRopstenAccountFromPrivateKeyFile(file string) (*Account, error) {
	_, key, err := PrivateKeyFromFile(file)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 3),
		reader.NewRopstenReader(),
		broadcaster.NewRopstenBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewRopstenTrezorAccount(path string, address string) (*Account, error) {
	signer, err := trezoreum.NewTrezorSignerGeneric(path, address, 3)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader.NewRopstenReader(),
		broadcaster.NewRopstenBroadcaster(),
		common.HexToAddress(address),
	}, nil
}

func NewRopstenLedgerAccount(path string, address string) (*Account, error) {
	signer, err := ledgereum.NewLedgerSignerGeneric(path, address, 3)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader.NewRopstenReader(),
		broadcaster.NewRopstenBroadcaster(),
		common.HexToAddress(address),
	}, nil
}
