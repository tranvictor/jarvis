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

// NewPrivateKeyAccount is used by external consumers of jarvis as a library
// (e.g. github.com/tranvictor/walletarmy); it has no callers inside this
// repo, so keep it even though dead-code analysis flags it.
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

// SignTypedDataV4 implements eth_signTypedData_v4. Trezor Model T / Safe
// devices require the structured EthereumSignTypedData protocol; hash-only
// signing is reserved for Gnosis Safe approvals via SignTypedDataHash.
func (self *Account) SignTypedDataV4(typedData *TypedDataV4) ([]byte, error) {
	sig, err := self.signer.SignTypedDataV4(&typedData.TypedData)
	if err != nil {
		return nil, fmt.Errorf("couldn't sign typed data: %w", err)
	}
	return sig, nil
}
