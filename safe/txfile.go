package safe

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// TxFile is the on-disk representation of a pending Safe transaction
// together with the signatures collected so far. It is what users pass
// between signers via email / Signal / Git / S3 when the chain has no
// Safe Transaction Service (or when they don't want to rely on it).
//
// The format is a superset of the Safe Transaction Service JSON response
// so the two can be used interchangeably: `safe`, `safeTxHash` and `tx`
// mirror the service's shape, and `signatures` is the same 65-byte r||s||v
// form the service uses. Numeric fields are encoded as strings so very
// large uint256s round-trip without JSON precision loss.
//
// ChainID is mandatory; jarvis cross-checks it against the active
// --network to prevent accidentally executing a SafeTx built for
// Ethereum mainnet on, say, BSC. Version (optional) is an operator hint
// and is not validated by jarvis.
type TxFile struct {
	Version    string              `json:"version,omitempty"`
	Safe       string              `json:"safe"`
	ChainID    uint64              `json:"chain_id"`
	Tx         TxFileSafeTx        `json:"tx"`
	SafeTxHash string              `json:"safe_tx_hash"`
	Sigs       []TxFileSignature   `json:"signatures,omitempty"`
}

// TxFileSafeTx mirrors SafeTx but with stringified big integers so the
// file survives any JSON library's number-precision limits and diffs
// cleanly in source control.
type TxFileSafeTx struct {
	To             string `json:"to"`
	Value          string `json:"value"`
	Data           string `json:"data"`
	Operation      uint8  `json:"operation"`
	SafeTxGas      string `json:"safe_tx_gas"`
	BaseGas        string `json:"base_gas"`
	GasPrice       string `json:"gas_price"`
	GasToken       string `json:"gas_token"`
	RefundReceiver string `json:"refund_receiver"`
	Nonce          string `json:"nonce"`
}

// TxFileSignature is one owner's confirmation, produced either off-chain
// via EIP-712 / eth_sign (65-byte ECDSA) or synthesised from
// OnChainApprovalSig when the owner approved on chain (v=0 marker).
// Kind is populated on read for readability only; it's derived from the
// sig's v byte and not trusted on load.
type TxFileSignature struct {
	Owner string `json:"owner"`
	Sig   string `json:"sig"`
	Kind  string `json:"kind,omitempty"` // "off-chain" | "on-chain" (informational)
}

// currentTxFileVersion is the format version we write. Readers accept
// any version; this exists so future changes can be rolled out cleanly.
const currentTxFileVersion = "jarvis-safe-txfile/v1"

// WriteTxFile serialises a pending Safe transaction to path. It
// overwrites any existing file; callers that want additive semantics
// should Read first, merge, and Write. The file is written as
// pretty-printed JSON with a trailing newline for git-friendliness.
func WriteTxFile(
	path, safeAddr string, chainID uint64,
	tx *SafeTx, hash [32]byte, sigs []OwnerSig,
) error {
	if tx == nil {
		return fmt.Errorf("nil SafeTx")
	}
	tf := TxFile{
		Version:    currentTxFileVersion,
		Safe:       common.HexToAddress(safeAddr).Hex(),
		ChainID:    chainID,
		Tx:         safeTxToFile(tx),
		SafeTxHash: "0x" + hex.EncodeToString(hash[:]),
	}
	for _, s := range sigs {
		kind := "off-chain"
		if IsOnChainApproval(s.Sig) {
			kind = "on-chain"
		}
		tf.Sigs = append(tf.Sigs, TxFileSignature{
			Owner: s.Owner.Hex(),
			Sig:   "0x" + hex.EncodeToString(s.Sig),
			Kind:  kind,
		})
	}
	data, err := json.MarshalIndent(&tf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tx file: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write tx file %s: %w", path, err)
	}
	return nil
}

// ReadTxFile loads a TxFile from path, verifying the basic shape. It
// does NOT recompute safeTxHash or verify signatures — callers should do
// that against the on-chain domainSeparator before acting on the file.
func ReadTxFile(path string) (*TxFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tx file %s: %w", path, err)
	}
	var tf TxFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parse tx file %s: %w", path, err)
	}
	if tf.Safe == "" {
		return nil, fmt.Errorf("tx file %s: missing safe address", path)
	}
	if tf.ChainID == 0 {
		return nil, fmt.Errorf("tx file %s: missing chain_id", path)
	}
	if tf.SafeTxHash == "" {
		return nil, fmt.Errorf("tx file %s: missing safe_tx_hash", path)
	}
	return &tf, nil
}

// ToPending converts the file into the same in-memory shape
// SignatureCollector.Get returns, so downstream code paths (execute,
// approve, info) can treat file-sourced and service-sourced pending txs
// uniformly.
func (f *TxFile) ToPending() (*PendingTx, error) {
	tx, err := fileToSafeTx(f.Tx)
	if err != nil {
		return nil, err
	}
	hash, err := hexToBytes32(f.SafeTxHash)
	if err != nil {
		return nil, fmt.Errorf("safe_tx_hash: %w", err)
	}
	sigs := make([]OwnerSig, 0, len(f.Sigs))
	for i, s := range f.Sigs {
		raw, err := hexToBytes(s.Sig)
		if err != nil {
			return nil, fmt.Errorf("sig %d: %w", i, err)
		}
		if len(raw) != 65 {
			return nil, fmt.Errorf("sig %d for %s: expected 65 bytes, got %d", i, s.Owner, len(raw))
		}
		sigs = append(sigs, OwnerSig{
			Owner: common.HexToAddress(s.Owner),
			Sig:   raw,
		})
	}
	return &PendingTx{
		Safe:       common.HexToAddress(f.Safe),
		SafeTx:     tx,
		SafeTxHash: hash,
		Sigs:       sigs,
	}, nil
}

// HasSigForOwner reports whether owner (case-insensitive) already has a
// signature recorded in the file.
func (f *TxFile) HasSigForOwner(owner common.Address) bool {
	for _, s := range f.Sigs {
		if strings.EqualFold(s.Owner, owner.Hex()) {
			return true
		}
	}
	return false
}

func safeTxToFile(tx *SafeTx) TxFileSafeTx {
	return TxFileSafeTx{
		To:             tx.To.Hex(),
		Value:          bigStr(tx.Value),
		Data:           "0x" + hex.EncodeToString(tx.Data),
		Operation:      uint8(tx.Operation),
		SafeTxGas:      bigStr(tx.SafeTxGas),
		BaseGas:        bigStr(tx.BaseGas),
		GasPrice:       bigStr(tx.GasPrice),
		GasToken:       tx.GasToken.Hex(),
		RefundReceiver: tx.RefundReceiver.Hex(),
		Nonce:          bigStr(tx.Nonce),
	}
}

func fileToSafeTx(tf TxFileSafeTx) (*SafeTx, error) {
	value, err := parseTxFileBig(tf.Value, "value")
	if err != nil {
		return nil, err
	}
	safeTxGas, err := parseTxFileBig(tf.SafeTxGas, "safe_tx_gas")
	if err != nil {
		return nil, err
	}
	baseGas, err := parseTxFileBig(tf.BaseGas, "base_gas")
	if err != nil {
		return nil, err
	}
	gasPrice, err := parseTxFileBig(tf.GasPrice, "gas_price")
	if err != nil {
		return nil, err
	}
	nonce, err := parseTxFileBig(tf.Nonce, "nonce")
	if err != nil {
		return nil, err
	}
	data, err := hexToBytes(tf.Data)
	if err != nil {
		return nil, fmt.Errorf("data: %w", err)
	}
	return &SafeTx{
		To:             common.HexToAddress(tf.To),
		Value:          value,
		Data:           data,
		Operation:      Operation(tf.Operation),
		SafeTxGas:      safeTxGas,
		BaseGas:        baseGas,
		GasPrice:       gasPrice,
		GasToken:       common.HexToAddress(tf.GasToken),
		RefundReceiver: common.HexToAddress(tf.RefundReceiver),
		Nonce:          nonce,
	}, nil
}

func bigStr(b *big.Int) string {
	if b == nil {
		return "0"
	}
	return b.String()
}

func parseTxFileBig(s, field string) (*big.Int, error) {
	if s == "" {
		return new(big.Int), nil
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("invalid %s: %q", field, s)
	}
	return v, nil
}
