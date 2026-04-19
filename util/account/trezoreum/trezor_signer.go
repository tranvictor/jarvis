package trezoreum

import (
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

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
	var err error
	if !self.deviceUnlocked {
		err = self.trezor.Unlock()
		if err != nil {
			return common.Address{}, tx, err
		}
		self.deviceUnlocked = true
	}
	return self.trezor.Sign(self.path, tx, chainId)
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
	if !self.deviceUnlocked {
		if err := self.trezor.Unlock(); err != nil {
			return nil, err
		}
		self.deviceUnlocked = true
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
