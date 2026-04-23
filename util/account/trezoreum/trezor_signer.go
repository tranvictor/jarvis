package trezoreum

import (
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// deviceWaitTimeout is how long we block Unlock() waiting for a
// Trezor to be physically connected/awake before giving up. Tuned
// for WalletConnect flows where the user often needs to plug the
// device in after the dApp already issued the request.
const deviceWaitTimeout = 90 * time.Second

// deviceWaitPoll is how often we retry enumeration while waiting.
const deviceWaitPoll = 2 * time.Second

type TrezorSigner struct {
	path           accounts.DerivationPath
	mu             sync.Mutex
	devmu          sync.Mutex
	deviceUnlocked bool
	trezor         Bridge
}

func (self *TrezorSigner) SignTx(
	tx *types.Transaction,
	chainId *big.Int,
) (common.Address, *types.Transaction, error) {
	self.mu.Lock()
	defer self.mu.Unlock()
	fmt.Printf("Going to proceed signing procedure\n")
	if err := self.ensureUnlocked(); err != nil {
		return common.Address{}, tx, err
	}
	return self.trezor.Sign(self.path, tx, chainId)
}

// ensureUnlocked unlocks the device on first use. If the device is
// not yet connected we poll for deviceWaitTimeout, printing progress,
// so users can plug the Trezor in after a dApp has already requested
// a signature.
func (self *TrezorSigner) ensureUnlocked() error {
	if self.deviceUnlocked {
		return nil
	}
	if err := unlockWithWait(self.trezor.Unlock, deviceWaitTimeout); err != nil {
		return err
	}
	self.deviceUnlocked = true
	return nil
}

// unlockWithWait repeatedly calls unlock until it succeeds, a
// non-"device not found" error happens, or timeout elapses. The
// user sees progress roughly every 10s.
func unlockWithWait(unlock func() error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	attempt := 0
	for {
		attempt++
		err := unlock()
		if err == nil {
			return nil
		}
		if !isTrezorNotConnected(err) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf(
				"trezor not connected after %s (last error: %w). Plug it in and try again",
				timeout, err)
		}
		if attempt == 1 {
			fmt.Printf(
				"Trezor not detected. Please connect and unlock it within %s...\n",
				timeout,
			)
		} else if attempt%5 == 0 {
			remaining := time.Until(deadline).Round(time.Second)
			fmt.Printf("  ...still waiting for Trezor (~%s left)\n", remaining)
		}
		time.Sleep(deviceWaitPoll)
	}
}

// isTrezorNotConnected reports whether err comes from the USB
// enumeration step failing to find a Trezor. We only retry these;
// any other error (PIN wrong, user reject, transport I/O) should
// surface immediately.
func isTrezorNotConnected(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"couldn't find any trezor devices",
		"couldn't open trezor device",
		"no such device",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// SignTypedDataHash signs a Gnosis-Safe EIP-712 digest. We try the native
// EthereumSignTypedHash opcode first and fall back to EthereumSignMessage
// (personal_sign) over the safeTxHash for older firmware.
//
// The returned signature is always Safe-compatible:
//   - native path: r||s||v with v in {27, 28} (canonical EIP-712).
//   - fallback path: r||s||(v+4) with v in {31, 32} (eth_sign code path).
func (self *TrezorSigner) SignTypedDataHash(
	domainSeparator, structHash [32]byte,
) ([]byte, error) {
	self.mu.Lock()
	defer self.mu.Unlock()
	if err := self.ensureUnlocked(); err != nil {
		return nil, err
	}

	fmt.Printf(
		"Asking Trezor to sign Safe EIP-712 hash (domain %s, message %s).\n",
		common.Bytes2Hex(domainSeparator[:]),
		common.Bytes2Hex(structHash[:]),
	)
	sig, err := self.trezor.SignTypedHash(self.path, domainSeparator, structHash)
	if err == nil {
		return sig, nil
	}
	if !shouldFallBackToPersonalSign(err) {
		return nil, err
	}
	fmt.Printf(
		"Trezor EthereumSignTypedHash failed (%s); falling back to personal_sign over the safeTxHash.\n",
		err,
	)

	digest := safeEIP712Digest(domainSeparator, structHash)
	sig, err = self.trezor.SignPersonalMessage(self.path, digest[:])
	if err != nil {
		return nil, err
	}
	if len(sig) != 65 {
		return nil, fmt.Errorf("trezor personal_sign returned %d bytes, want 65", len(sig))
	}
	sig[64] += 4
	return sig, nil
}

// SignPersonalMessage signs message with the EIP-191 personal_sign
// prefix via the Trezor's EthereumSignMessage opcode. Trezor firmware
// applies the "\x19Ethereum Signed Message:\n<len>" prefix internally,
// so callers pass the raw message bytes. v is normalised to {27, 28}
// inside Trezoreum.SignPersonalMessage.
func (self *TrezorSigner) SignPersonalMessage(message []byte) ([]byte, error) {
	self.mu.Lock()
	defer self.mu.Unlock()
	if err := self.ensureUnlocked(); err != nil {
		return nil, err
	}
	return self.trezor.SignPersonalMessage(self.path, message)
}

// shouldFallBackToPersonalSign returns true when the failure indicates the
// firmware does not implement EthereumSignTypedHash, rather than a user
// rejection or a transport error.
func shouldFallBackToPersonalSign(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"unexpected message",
		"failure_unexpectedmessage",
		"unsupported",
		"not supported",
		"unknown message",
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
	}, nil
}
