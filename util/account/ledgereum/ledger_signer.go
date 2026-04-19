package ledgereum

import (
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	kusb "github.com/tranvictor/jarvis/util/account/usb"
)

// minLedgerEthAppVersionForEIP712 is the lowest Ethereum app version known to
// implement opcode 0x0C (SIGN_ETH_EIP_712 with precomputed hashes). For older
// firmware we transparently fall back to opcode 0x08 (SIGN_PERSONAL_MESSAGE)
// signing the raw safeTxHash, with v += 4 for Safe's eth_sign code path.
var minLedgerEthAppVersionForEIP712 = [3]byte{1, 9, 19}

func ledgerVersionAtLeast(have, min [3]byte) bool {
	for i := 0; i < 3; i++ {
		if have[i] != min[i] {
			return have[i] > min[i]
		}
	}
	return true
}

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
) (common.Address, *types.Transaction, error) {
	self.mu.Lock()
	defer self.mu.Unlock()
	fmt.Printf("Going to proceed signing procedure\n")
	var err error
	if !self.deviceUnlocked {
		err = self.Unlock()
		if err != nil {
			return common.Address{}, tx, err
		}
	}
	return self.driver.ledgerSign(self.path, tx, chainId)
}

// SignTypedDataHash signs a Gnosis-Safe EIP-712 digest. We prefer the native
// EIP-712-by-hash opcode (0x0C) when the Ledger Ethereum app is recent enough,
// and fall back to personal_sign over the digest for older firmware.
//
// Both paths return a Safe-compatible 65-byte signature: the EIP-712 path
// yields v in {27, 28}, while the personal_sign fallback returns v in {31, 32}
// (Safe subtracts 4 and re-hashes as a personal message before recovery).
func (self *LedgerSigner) SignTypedDataHash(
	domainSeparator, structHash [32]byte,
) ([]byte, error) {
	self.mu.Lock()
	defer self.mu.Unlock()
	if !self.deviceUnlocked {
		if err := self.Unlock(); err != nil {
			return nil, err
		}
	}

	if ledgerVersionAtLeast(self.driver.version, minLedgerEthAppVersionForEIP712) {
		fmt.Printf(
			"Ledger Ethereum app v%d.%d.%d supports EIP-712 by hash; please confirm on the device.\n",
			self.driver.version[0], self.driver.version[1], self.driver.version[2],
		)
		sig, err := self.driver.ledgerSignTypedHash(self.path, domainSeparator, structHash)
		if err == nil {
			return sig, nil
		}
		// Some devices (clone HW, dev builds, app blind-sign disabled) advertise
		// a recent version but still reject opcode 0x0C. Fall through.
		fmt.Printf("Ledger EIP-712 (0x0C) signing failed: %s\n", err)
		if !shouldFallBackToPersonalSign(err) {
			return nil, err
		}
		fmt.Printf("Falling back to personal_sign (eth_sign) over the safeTxHash.\n")
	} else {
		fmt.Printf(
			"Ledger Ethereum app v%d.%d.%d does not support EIP-712 by hash; "+
				"falling back to personal_sign (please enable blind-signing on the device).\n",
			self.driver.version[0], self.driver.version[1], self.driver.version[2],
		)
	}

	digest := safeEIP712Digest(domainSeparator, structHash)
	sig, err := self.driver.ledgerSignPersonalMessage(self.path, digest[:])
	if err != nil {
		return nil, err
	}
	if len(sig) != 65 {
		return nil, fmt.Errorf("ledger personal_sign returned %d bytes, want 65", len(sig))
	}
	// Tell Safe this signature came through the eth_sign code path.
	sig[64] += 4
	return sig, nil
}

// shouldFallBackToPersonalSign returns true for failures that look like
// "device refused / unsupported instruction" rather than user rejection.
func shouldFallBackToPersonalSign(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"unknown instruction",
		"unsupported",
		"6d00", // INS not supported
		"6e00", // CLA not supported
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// safeEIP712Digest mirrors account.safeEIP712Digest; duplicated here to avoid
// an import cycle (this package is imported by util/account).
func safeEIP712Digest(domainSeparator, structHash [32]byte) [32]byte {
	buf := make([]byte, 0, 2+32+32)
	buf = append(buf, 0x19, 0x01)
	buf = append(buf, domainSeparator[:]...)
	buf = append(buf, structHash[:]...)
	var out [32]byte
	copy(out[:], crypto.Keccak256(buf))
	return out
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
