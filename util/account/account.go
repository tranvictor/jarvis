package account

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/tranvictor/jarvis/util/account/ledgereum"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
)

type Account struct {
	signer  Signer
	address common.Address
}

func NewPrivateKeyAccount(privateKey string) (*Account, error) {
	key, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewKeystoreAccount(file string, password string) (*Account, error) {
	_, key, err := PrivateKeyFromKeystore(file, password)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key),
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewTrezorAccount(path string, address string) (*Account, error) {
	signer, err := trezoreum.NewTrezorSigner(path, address)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		common.HexToAddress(address),
	}, nil
}

func NewLedgerAccount(path string, address string) (*Account, error) {
	signer, err := ledgereum.NewLedgerSigner(path, address)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		common.HexToAddress(address),
	}, nil
}

func (self *Account) Address() common.Address {
	return self.address
}

func (self *Account) AddressHex() string {
	return self.address.Hex()
}

func (self *Account) SignTx(
	tx *types.Transaction,
	chainId *big.Int,
) (common.Address, *types.Transaction, error) {
	addr, signedTx, err := self.signer.SignTx(tx, chainId)
	if err != nil {
		return addr, tx, fmt.Errorf("Couldn't sign the tx: %s", err)
	}
	return addr, signedTx, nil
}

// SignSafeHash returns a 65-byte Safe-compatible signature over the EIP-712
// digest keccak256(0x19 0x01 || domainSeparator || structHash).
//
// The encoding of v in the returned signature depends on the underlying
// signer:
//   - private key / keystore (KeySigner) and Ledger app >= 1.9.19 / Trezor T
//     with EthereumSignTypedHash use canonical EIP-712 (v in {27, 28}).
//   - Older Ledger / Trezor firmware fall back to personal_sign over the
//     digest and return v in {31, 32} (= original v + 4), which Safe also
//     accepts via its eth_sign code path.
func (self *Account) SignSafeHash(domainSeparator, structHash [32]byte) ([]byte, error) {
	sig, err := self.signer.SignTypedDataHash(domainSeparator, structHash)
	if err != nil {
		return nil, fmt.Errorf("couldn't sign safe hash: %w", err)
	}
	return sig, nil
}

// SignPersonalMessage signs message with the EIP-191 personal_sign
// prefix. The 65-byte signature uses canonical v in {27, 28}.
//
// Used by walletconnect's EOA gateway to service personal_sign requests
// from dApps; unrelated to the Safe eth_sign fallback path which
// deliberately returns v + 4.
func (self *Account) SignPersonalMessage(message []byte) ([]byte, error) {
	sig, err := self.signer.SignPersonalMessage(message)
	if err != nil {
		return nil, fmt.Errorf("couldn't sign personal message: %w", err)
	}
	return sig, nil
}

// SignTypedDataV4 implements eth_signTypedData_v4 by decomposing the
// typed data into a domain separator + struct hash and handing the
// pair to the underlying Signer. This reuses the same device code path
// that already works for Safe EIP-712 signing (both Ledger opcode 0x0C
// and Trezor EthereumSignTypedHash), so the only device-specific
// requirement is that the firmware supports EIP-712-by-hash.
//
// typedData is the already-parsed EIP-712 payload; the caller is
// expected to have validated the shape. The returned signature is a
// canonical 65-byte r||s||v with v in {27, 28}.
func (self *Account) SignTypedDataV4(typedData *TypedDataV4) ([]byte, error) {
	domainSep, structHash, err := typedData.Hashes()
	if err != nil {
		return nil, fmt.Errorf("hash typed data: %w", err)
	}
	sig, err := self.signer.SignTypedDataHash(domainSep, structHash)
	if err != nil {
		return nil, fmt.Errorf("couldn't sign typed data: %w", err)
	}
	// SignTypedDataHash for Safe returns v in {27, 28} for the
	// canonical path and v in {31, 32} for the eth_sign fallback.
	// Neither variant is strictly speaking "wrong" for
	// eth_signTypedData_v4 — Safe's odd +4 convention is specific to
	// Safe's signature multiplexing — but most dApps verify via
	// ecrecover assuming v in {27, 28}. Normalise here.
	if len(sig) == 65 && (sig[64] == 31 || sig[64] == 32) {
		sig[64] -= 4
	}
	return sig, nil
}
