package trezoreum

import (
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/core/types"
)

type TrezorSigner struct {
	path           accounts.DerivationPath
	mu             sync.Mutex
	devmu          sync.Mutex
	deviceUnlocked bool
	trezor         Bridge
	chainID        int64
}

func (self *TrezorSigner) SignTx(tx *types.Transaction) (*types.Transaction, error) {
	self.mu.Lock()
	defer self.mu.Unlock()
	fmt.Printf("Going to proceed signing procedure\n")
	var err error
	if !self.deviceUnlocked {
		err = self.trezor.Unlock()
		if err != nil {
			return tx, err
		}
		self.deviceUnlocked = true
	}
	_, tx, err = self.trezor.Sign(self.path, tx, big.NewInt(self.chainID))
	return tx, err
}

func NewRopstenTrezorSigner(path string, address string) (*TrezorSigner, error) {
	p, err := accounts.ParseDerivationPath(path)
	if err != nil {
		return nil, err
	}
	trezor, err := NewTrezoreum()
	if err != nil {
		return nil, err
	}
	return &TrezorSigner{
		p,
		sync.Mutex{},
		sync.Mutex{},
		false,
		trezor,
		1,
	}, nil
}

func NewTrezorSignerGeneric(path string, address string, chainID int64) (*TrezorSigner, error) {
	p, err := accounts.ParseDerivationPath(path)
	if err != nil {
		return nil, err
	}
	trezor, err := NewTrezoreum()
	if err != nil {
		return nil, err
	}
	return &TrezorSigner{
		p,
		sync.Mutex{},
		sync.Mutex{},
		false,
		trezor,
		chainID,
	}, nil
}

func NewTrezorSigner(path string, address string) (*TrezorSigner, error) {
	p, err := accounts.ParseDerivationPath(path)
	if err != nil {
		return nil, err
	}
	trezor, err := NewTrezoreum()
	if err != nil {
		return nil, err
	}
	return &TrezorSigner{
		p,
		sync.Mutex{},
		sync.Mutex{},
		false,
		trezor,
		1,
	}, nil
}

func NewTrezorTomoSigner(path string, address string) (*TrezorSigner, error) {
	p, err := accounts.ParseDerivationPath(path)
	if err != nil {
		return nil, err
	}
	trezor, err := NewTrezoreum()
	if err != nil {
		return nil, err
	}
	return &TrezorSigner{
		p,
		sync.Mutex{},
		sync.Mutex{},
		false,
		trezor,
		88,
	}, nil
}
