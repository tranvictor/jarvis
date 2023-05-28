package ledgereum

import (
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	kusb "github.com/karalabe/usb"
)

const (
	LEDGER_VENDOR_ID   uint16 = 0x2c97
	LEDGER_USAGE_ID    uint16 = 0xffa0
	LEDGER_ENDPOINT_ID int    = 0
)

var LEDGER_PRODUCT_IDS []uint16 = []uint16{
	// Original product IDs
	0x0000, /* Ledger Blue */
	0x0001, /* Ledger Nano S */
	0x0004, /* Ledger Nano X */

	// Upcoming product IDs: https://www.ledger.com/2019/05/17/windows-10-update-sunsetting-u2f-tunnel-transport-for-ledger-devices/
	0x0015, /* HID + U2F + WebUSB Ledger Blue */
	0x1015, /* HID + U2F + WebUSB Ledger Nano S */
	0x4015, /* HID + U2F + WebUSB Ledger Nano X */
	0x0011, /* HID + WebUSB Ledger Blue */
	0x1011, /* HID + WebUSB Ledger Nano S */
	0x4011, /* HID + WebUSB Ledger Nano X */
}

type Ledgereum struct {
	driver         *ledgerDriver
	device         kusb.Device
	devmu          sync.Mutex
	deviceUnlocked bool
}

func NewLedgereum() (*Ledgereum, error) {
	return &Ledgereum{
		newLedgerDriver(),
		nil,
		sync.Mutex{},
		false,
	}, nil
}

func (self *Ledgereum) Unlock() error {
	self.devmu.Lock()
	defer self.devmu.Unlock()
	// infos, err := kusb.Enumerate(LEDGER_VENDOR_ID, 0)
	infos, err := kusb.Enumerate(0, 0)
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		return fmt.Errorf("Ledger device is not found")
	} else {
		for i, info := range infos {
			fmt.Printf("Device %d: Vendor ID: %d, %v\n", i, info.VendorID, info)
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

func (self *Ledgereum) Derive(path accounts.DerivationPath) (common.Address, error) {
	return self.driver.Derive(path)
}

func (self *Ledgereum) Sign(path accounts.DerivationPath, tx *types.Transaction, chainID *big.Int) (common.Address, *types.Transaction, error) {
	return self.driver.SignTx(path, tx, chainID)
}
