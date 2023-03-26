package account

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tranvictor/jarvis/util/account/ledgereum"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
	"github.com/tranvictor/jarvis/util/broadcaster"
	"github.com/tranvictor/jarvis/util/reader"
)

func NewBSCTestnetAccountFromKeystore(file string, password string) (*Account, error) {
	_, key, err := PrivateKeyFromKeystore(file, password)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 97),
		reader.NewBSCTestnetReader(),
		broadcaster.NewBSCTestnetBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewBSCTestnetAccountFromPrivateKey(hex string) (*Account, error) {
	_, key, err := PrivateKeyFromHex(hex)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 97),
		reader.NewBSCTestnetReader(),
		broadcaster.NewBSCTestnetBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewBSCTestnetAccountFromPrivateKeyFile(file string) (*Account, error) {
	_, key, err := PrivateKeyFromFile(file)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, 97),
		reader.NewBSCTestnetReader(),
		broadcaster.NewBSCTestnetBroadcaster(),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewBSCTestnetTrezorAccount(path string, address string) (*Account, error) {
	signer, err := trezoreum.NewTrezorSignerGeneric(path, address, 97)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader.NewBSCTestnetReader(),
		broadcaster.NewBSCTestnetBroadcaster(),
		common.HexToAddress(address),
	}, nil
}

func NewBSCTestnetLedgerAccount(path string, address string) (*Account, error) {
	signer, err := ledgereum.NewLedgerSignerGeneric(path, address, 97)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader.NewBSCTestnetReader(),
		broadcaster.NewBSCTestnetBroadcaster(),
		common.HexToAddress(address),
	}, nil
}
