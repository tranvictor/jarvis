package safe

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Operation is the call type executed by the Safe.
type Operation uint8

const (
	OpCall         Operation = 0
	OpDelegateCall Operation = 1
)

// EIP-712 type hash for SafeTx as defined by Safe v1.3.0+:
//
//	keccak256("SafeTx(address to,uint256 value,bytes data,uint8 operation,uint256 safeTxGas,uint256 baseGas,uint256 gasPrice,address gasToken,address refundReceiver,uint256 nonce)")
var safeTxTypeHash = crypto.Keccak256(
	[]byte("SafeTx(address to,uint256 value,bytes data,uint8 operation,uint256 safeTxGas,uint256 baseGas,uint256 gasPrice,address gasToken,address refundReceiver,uint256 nonce)"),
)

// SafeTx mirrors the parameters of GnosisSafe.execTransaction(...). All
// numeric fields are stored as *big.Int so we don't lose precision when
// round-tripping through the Safe Transaction Service (which serialises
// every uint256 as a JSON string).
type SafeTx struct {
	To             common.Address
	Value          *big.Int
	Data           []byte
	Operation      Operation
	SafeTxGas      *big.Int
	BaseGas        *big.Int
	GasPrice       *big.Int
	GasToken       common.Address
	RefundReceiver common.Address
	Nonce          *big.Int
}

// NewSafeTx returns a SafeTx with all-zero gas/refund fields, suitable for a
// "regular" Safe transaction signed off-chain and executed by any owner.
func NewSafeTx(to common.Address, value *big.Int, data []byte, op Operation, nonce uint64) *SafeTx {
	return &SafeTx{
		To:             to,
		Value:          orZero(value),
		Data:           data,
		Operation:      op,
		SafeTxGas:      big.NewInt(0),
		BaseGas:        big.NewInt(0),
		GasPrice:       big.NewInt(0),
		GasToken:       common.Address{},
		RefundReceiver: common.Address{},
		Nonce:          new(big.Int).SetUint64(nonce),
	}
}

func orZero(x *big.Int) *big.Int {
	if x == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(x)
}

// StructHash returns the EIP-712 struct hash of the SafeTx (without the
// 0x1901 prefix and domain separator). The result matches what Safe contracts
// hash internally before signing/verification.
func (t *SafeTx) StructHash() [32]byte {
	enc := bytes.Buffer{}
	enc.Write(safeTxTypeHash)
	enc.Write(common.LeftPadBytes(t.To.Bytes(), 32))
	enc.Write(common.LeftPadBytes(t.Value.Bytes(), 32))
	enc.Write(crypto.Keccak256(t.Data))
	enc.Write(common.LeftPadBytes([]byte{byte(t.Operation)}, 32))
	enc.Write(common.LeftPadBytes(t.SafeTxGas.Bytes(), 32))
	enc.Write(common.LeftPadBytes(t.BaseGas.Bytes(), 32))
	enc.Write(common.LeftPadBytes(t.GasPrice.Bytes(), 32))
	enc.Write(common.LeftPadBytes(t.GasToken.Bytes(), 32))
	enc.Write(common.LeftPadBytes(t.RefundReceiver.Bytes(), 32))
	enc.Write(common.LeftPadBytes(t.Nonce.Bytes(), 32))

	var out [32]byte
	copy(out[:], crypto.Keccak256(enc.Bytes()))
	return out
}

// SafeTxHash returns the EIP-712 digest signed by Safe owners:
//
//	keccak256(0x19 || 0x01 || domainSeparator || structHash)
func (t *SafeTx) SafeTxHash(domainSeparator [32]byte) [32]byte {
	mh := t.StructHash()
	enc := make([]byte, 0, 2+32+32)
	enc = append(enc, 0x19, 0x01)
	enc = append(enc, domainSeparator[:]...)
	enc = append(enc, mh[:]...)
	var out [32]byte
	copy(out[:], crypto.Keccak256(enc))
	return out
}

// OwnerSig pairs a 65-byte Safe-compatible signature with the address of the
// owner who produced it. Callers obtain Sig from Account.SignSafeHash, which
// already encodes the device-specific v adjustment Safe expects.
//
// For owners who approved on-chain via approveHash(safeTxHash), use
// OnChainApprovalSig(owner) to build a Sig. That variant carries no
// cryptographic material; GnosisSafe.checkSignatures recognises it by the
// sentinel v=0 byte and verifies it against the approvedHashes mapping.
type OwnerSig struct {
	Owner common.Address
	Sig   []byte // 65 bytes: r (32) || s (32) || v (1)
}

// OnChainApprovalSig returns a 65-byte "pre-approved hash" marker for owner
// in the format GnosisSafe.checkSignatures consumes when v == 0:
//
//	r = left-padded owner address (32 bytes)
//	s = zero (32 bytes)
//	v = 0 (1 byte)
//
// The Safe contract detects the v=0 sentinel and, instead of running
// ecrecover, reads approvedHashes[owner][safeTxHash] from storage. An
// execution that includes this marker will therefore revert with GS025
// unless the owner has actually called approveHash(...) on chain.
func OnChainApprovalSig(owner common.Address) OwnerSig {
	sig := make([]byte, 65)
	copy(sig[12:32], owner.Bytes())
	// s (sig[32:64]) stays zero; v (sig[64]) stays zero — the sentinel.
	return OwnerSig{Owner: owner, Sig: sig}
}

// IsOnChainApproval reports whether sig is the v=0 pre-approved-hash marker
// produced by OnChainApprovalSig (as opposed to an ECDSA / eth_sign sig).
// Callers use this to annotate UI output ("on-chain") and to skip the self
// already-signed check for the on-chain-approve path.
func IsOnChainApproval(sig []byte) bool {
	return len(sig) == 65 && sig[64] == 0
}

// EncodeSignatures returns the `signatures` blob in the exact layout
// GnosisSafe.execTransaction(...) consumes: signatures sorted ascending by
// owner address, each laid out as r||s||v. The Safe contract iterates the
// blob in 65-byte strides, so duplicate owners or wrong ordering will fail
// signature verification with "GS026" / "Invalid owner provided" type errors.
func EncodeSignatures(sigs []OwnerSig) ([]byte, error) {
	if len(sigs) == 0 {
		return nil, fmt.Errorf("no signatures provided")
	}
	sorted := make([]OwnerSig, len(sigs))
	copy(sorted, sigs)
	sort.SliceStable(sorted, func(i, j int) bool {
		return bytes.Compare(sorted[i].Owner.Bytes(), sorted[j].Owner.Bytes()) < 0
	})

	out := make([]byte, 0, 65*len(sorted))
	for i, s := range sorted {
		if len(s.Sig) != 65 {
			return nil, fmt.Errorf("sig %d for %s has length %d, want 65", i, s.Owner.Hex(), len(s.Sig))
		}
		if i > 0 && sorted[i-1].Owner == s.Owner {
			return nil, fmt.Errorf("duplicate owner %s in signature set", s.Owner.Hex())
		}
		out = append(out, s.Sig...)
	}
	return out, nil
}

// EncodeSignaturesHex is a convenience wrapper returning a 0x-prefixed
// lowercase hex string for use in JSON payloads.
func EncodeSignaturesHex(sigs []OwnerSig) (string, error) {
	raw, err := EncodeSignatures(sigs)
	if err != nil {
		return "", err
	}
	return "0x" + strings.ToLower(hex.EncodeToString(raw)), nil
}
