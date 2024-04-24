package ledgereum

import (
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/core/types"

	kusb "github.com/tranvictor/jarvis/util/account/usb"
)

type LedgerSigner struct {
	path           accounts.DerivationPath
	driver         *ledgerDriver
	device         kusb.Device
	deviceUnlocked bool
	mu             sync.Mutex
	devmu          sync.Mutex
}

func (self *LedgerSigner) Unlock() error {
	self.devmu.Lock()
	defer self.devmu.Unlock()
	infos, err := kusb.Enumerate(LEDGER_VENDOR_ID, 0)
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		return fmt.Errorf("Ledger device is not found")
	} else {
		for _, info := range infos {
			for _, id := range LEDGER_PRODUCT_IDS {
				// Windows and Macos use UsageID matching, Linux uses Interface matching
				if info.ProductID == id && (info.UsagePage == LEDGER_USAGE_ID || info.Interface == LEDGER_ENDPOINT_ID) {
					self.device, err = info.Open()
					if err != nil {
						return err
					}
					if err = self.driver.Open(self.device, ""); err != nil {
						return err
					}
					break
				}
			}
		}
	}
	self.deviceUnlocked = true
	return nil
}

func (self *LedgerSigner) SignTx(
	tx *types.Transaction,
	chainId *big.Int,
) (*types.Transaction, error) {
	self.mu.Lock()
	defer self.mu.Unlock()
	fmt.Printf("Going to proceed signing procedure\n")
	var err error
	if !self.deviceUnlocked {
		err = self.Unlock()
		if err != nil {
			return tx, err
		}
	}
	_, tx, err = self.driver.ledgerSign(self.path, tx, chainId)
	return tx, err
}

func NewLedgerSigner(path string, address string) (*LedgerSigner, error) {
	p, err := accounts.ParseDerivationPath(path)
	if err != nil {
		return nil, err
	}
	return &LedgerSigner{
		p,
		newLedgerDriver(),
		nil,
		false,
		sync.Mutex{},
		sync.Mutex{},
	}, nil
}
