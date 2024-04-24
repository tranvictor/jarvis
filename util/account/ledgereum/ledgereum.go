package ledgereum

import (
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	. "github.com/tranvictor/jarvis/common"
	kusb "github.com/tranvictor/jarvis/util/account/usb"
)

const (
	LEDGER_VENDOR_ID   uint16 = 0x2c97
	LEDGER_USAGE_ID    uint16 = 0xffa0
	LEDGER_ENDPOINT_ID int    = 0
)

var LEDGER_PRODUCT_IDS []uint16 = []uint16{
	// Device definitions taken from
	// https://github.com/LedgerHQ/ledger-live/blob/38012bc8899e0f07149ea9cfe7e64b2c146bc92b/libs/ledgerjs/packages/devices/src/index.ts

	// Original product IDs
	0x0000, /* Ledger Blue */
	0x0001, /* Ledger Nano S */
	0x0004, /* Ledger Nano X */
	0x0005, /* Ledger Nano S Plus */
	0x0006, /* Ledger Nano FTS */

	0x0015, /* HID + U2F + WebUSB Ledger Blue */
	0x1015, /* HID + U2F + WebUSB Ledger Nano S */
	0x4015, /* HID + U2F + WebUSB Ledger Nano X */
	0x5015, /* HID + U2F + WebUSB Ledger Nano S Plus */
	0x6015, /* HID + U2F + WebUSB Ledger Nano FTS */

	0x0011, /* HID + WebUSB Ledger Blue */
	0x1011, /* HID + WebUSB Ledger Nano S */
	0x4011, /* HID + WebUSB Ledger Nano X */
	0x5011, /* HID + WebUSB Ledger Nano S Plus */
	0x6011, /* HID + WebUSB Ledger Nano FTS */
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
			DebugPrintf("Device %d: Vendor ID: %d, %v\n", i, info.VendorID, info)
			for _, id := range LEDGER_PRODUCT_IDS {
				// Windows and Macos use UsageID matching, Linux uses Interface matching
				if info.ProductID == id && (info.UsagePage == LEDGER_USAGE_ID || info.Interface == LEDGER_ENDPOINT_ID) {
					DebugPrintf("setting up device instance...")
					self.device, err = info.Open()
					DebugPrintf("done. Instance: %v, err: %v\n", self.device, err)
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

	if self.device == nil {
		return fmt.Errorf("Ledger device is not found")
	}

	self.deviceUnlocked = true
	return nil
}

func (self *Ledgereum) Derive(path accounts.DerivationPath) (common.Address, error) {
	return self.driver.Derive(path)
}

func (self *Ledgereum) Sign(
	path accounts.DerivationPath,
	tx *types.Transaction,
	chainID *big.Int,
) (common.Address, *types.Transaction, error) {
	return self.driver.SignTx(path, tx, chainID)
}
