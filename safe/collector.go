package safe

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/safe/txservice"
)

// PendingTx is the storage-backend-agnostic view of a pending Safe tx.
// It is what jarvis commands display, sign, or hand to execTransaction —
// independent of whether the source is the Safe Transaction Service, an
// on-chain approveHash registry, or a future local file collector.
type PendingTx struct {
	Safe       common.Address
	SafeTx     *SafeTx
	SafeTxHash [32]byte
	IsExecuted bool
	// Sigs is the set of owner signatures collected so far. Each signature is
	// already in Safe-compatible 65-byte r||s||v form (with v adjusted for
	// eth_sign-style signers when needed).
	Sigs []OwnerSig
}

// SignatureCollector abstracts the storage backend that holds pending Safe
// transactions and their off-chain or pre-execution signatures.
//
// In v1 only TxServiceCollector is implemented (collecting via the Safe
// Transaction Service REST API). The interface intentionally mirrors that
// service's verbs so future on-chain (approveHash) or file-backed
// collectors can be slotted in without changing cmd/safe.go.
type SignatureCollector interface {
	// Propose submits a brand-new Safe transaction together with the
	// proposing owner's signature.
	Propose(safe common.Address, tx *SafeTx, hash [32]byte, proposer common.Address, sig []byte) error

	// Confirm appends an additional owner signature for an existing
	// pending tx identified by hash.
	Confirm(hash [32]byte, owner common.Address, sig []byte) error

	// Get fetches a pending transaction by hash, including all
	// signatures collected so far.
	Get(hash [32]byte) (*PendingTx, error)

	// FindByNonce returns the queued tx (if any) for a given Safe + Safe
	// nonce. Useful when the user knows the nonce but not the hash.
	FindByNonce(safe common.Address, nonce uint64) (*PendingTx, error)

	// ListPending returns every queued (not yet executed) multisig tx the
	// backend knows about for safe, ordered by nonce ascending. Used by
	// the CLI to auto-pick when only one pending tx exists, or to print a
	// menu when several do.
	ListPending(safe common.Address) ([]*PendingTx, error)
}

// TxServiceCollector implements SignatureCollector using the Safe
// Transaction Service REST API.
type TxServiceCollector struct {
	Client *txservice.Client
}

// NewTxServiceCollector returns a SignatureCollector backed by the canonical
// Safe Transaction Service for the given chain ID, honoring environment
// overrides (see safe/txservice.URLForChain).
func NewTxServiceCollector(chainID uint64) (*TxServiceCollector, error) {
	c, err := txservice.NewClient(chainID)
	if err != nil {
		return nil, err
	}
	return &TxServiceCollector{Client: c}, nil
}

func (c *TxServiceCollector) Propose(
	safe common.Address,
	tx *SafeTx,
	hash [32]byte,
	proposer common.Address,
	sig []byte,
) error {
	if len(sig) != 65 {
		return fmt.Errorf("propose sig must be 65 bytes, got %d", len(sig))
	}
	req := &txservice.ProposeTxRequest{
		To:                      tx.To.Hex(),
		Value:                   tx.Value.String(),
		Data:                    "0x" + hex.EncodeToString(tx.Data),
		Operation:               uint8(tx.Operation),
		SafeTxGas:               tx.SafeTxGas.String(),
		BaseGas:                 tx.BaseGas.String(),
		GasPrice:                tx.GasPrice.String(),
		GasToken:                tx.GasToken.Hex(),
		RefundReceiver:          tx.RefundReceiver.Hex(),
		Nonce:                   tx.Nonce.String(),
		ContractTransactionHash: "0x" + hex.EncodeToString(hash[:]),
		Sender:                  proposer.Hex(),
		Signature:               "0x" + hex.EncodeToString(sig),
		Origin:                  "jarvis",
	}
	if len(tx.Data) == 0 {
		req.Data = "0x"
	}
	return c.Client.Propose(safe.Hex(), req)
}

func (c *TxServiceCollector) Confirm(
	hash [32]byte,
	_ common.Address,
	sig []byte,
) error {
	if len(sig) != 65 {
		return fmt.Errorf("confirm sig must be 65 bytes, got %d", len(sig))
	}
	hashHex := "0x" + hex.EncodeToString(hash[:])
	return c.Client.Confirm(hashHex, &txservice.ConfirmRequest{
		Signature: "0x" + hex.EncodeToString(sig),
	})
}

func (c *TxServiceCollector) Get(hash [32]byte) (*PendingTx, error) {
	hashHex := "0x" + hex.EncodeToString(hash[:])
	mt, err := c.Client.GetTx(hashHex)
	if err != nil {
		return nil, err
	}
	return c.toPending(mt, hash)
}

func (c *TxServiceCollector) ListPending(safe common.Address) ([]*PendingTx, error) {
	mts, err := c.Client.ListPending(safe.Hex(), nil)
	if err != nil {
		return nil, err
	}
	out := make([]*PendingTx, 0, len(mts))
	for i := range mts {
		mt := mts[i]
		hashBytes, err := hexToBytes32(mt.SafeTxHash)
		if err != nil {
			return nil, fmt.Errorf("invalid safeTxHash from service: %w", err)
		}
		p, err := c.toPending(&mt, hashBytes)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (c *TxServiceCollector) FindByNonce(safe common.Address, nonce uint64) (*PendingTx, error) {
	mt, err := c.Client.FindByNonce(safe.Hex(), nonce)
	if err != nil {
		return nil, err
	}
	if mt == nil {
		return nil, nil
	}
	hashBytes, err := hexToBytes32(mt.SafeTxHash)
	if err != nil {
		return nil, fmt.Errorf("invalid safeTxHash from service: %w", err)
	}
	return c.toPending(mt, hashBytes)
}

func (c *TxServiceCollector) toPending(mt *txservice.MultisigTx, hash [32]byte) (*PendingTx, error) {
	value, err := parseBigDecimal(mt.Value.String(), "value")
	if err != nil {
		return nil, err
	}
	safeTxGas, err := parseBigDecimal(mt.SafeTxGas.String(), "safeTxGas")
	if err != nil {
		return nil, err
	}
	baseGas, err := parseBigDecimal(mt.BaseGas.String(), "baseGas")
	if err != nil {
		return nil, err
	}
	gasPrice, err := parseBigDecimal(mt.GasPrice.String(), "gasPrice")
	if err != nil {
		return nil, err
	}

	var data []byte
	if mt.Data != nil {
		var err error
		data, err = hexToBytes(*mt.Data)
		if err != nil {
			return nil, fmt.Errorf("invalid data: %w", err)
		}
	}

	tx := &SafeTx{
		To:             common.HexToAddress(mt.To),
		Value:          value,
		Data:           data,
		Operation:      Operation(mt.Operation),
		SafeTxGas:      safeTxGas,
		BaseGas:        baseGas,
		GasPrice:       gasPrice,
		GasToken:       common.HexToAddress(mt.GasToken),
		RefundReceiver: common.HexToAddress(mt.RefundReceiver),
		Nonce:          new(big.Int).SetUint64(mt.Nonce),
	}

	sigs := make([]OwnerSig, 0, len(mt.Confirmations))
	for _, cf := range mt.Confirmations {
		raw, err := hexToBytes(cf.Signature)
		if err != nil {
			return nil, fmt.Errorf("confirmation by %s: invalid signature: %w", cf.Owner, err)
		}
		// Some signature types reported by the service (e.g. APPROVED_HASH,
		// CONTRACT_SIGNATURE) are not 65-byte ECDSA — for v1 we only consume
		// EOA / ETH_SIGN style signatures and skip the rest. Execution will
		// fail loudly later if the threshold can't be met.
		if len(raw) != 65 {
			continue
		}
		sigs = append(sigs, OwnerSig{
			Owner: common.HexToAddress(cf.Owner),
			Sig:   raw,
		})
	}

	return &PendingTx{
		Safe:       common.HexToAddress(mt.Safe),
		SafeTx:     tx,
		SafeTxHash: hash,
		IsExecuted: mt.IsExecuted,
		Sigs:       sigs,
	}, nil
}

// parseBigDecimal parses a possibly-empty decimal integer string into *big.Int.
// Empty strings (the JSON-null marker that numString collapses to) decode as
// zero so callers can treat absent gas fields as "no value" without special-casing.
func parseBigDecimal(s, field string) (*big.Int, error) {
	if s == "" {
		return new(big.Int), nil
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("invalid %s: %s", field, s)
	}
	return v, nil
}

func hexToBytes(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	if s == "" {
		return []byte{}, nil
	}
	return hex.DecodeString(s)
}

func hexToBytes32(s string) ([32]byte, error) {
	b, err := hexToBytes(s)
	if err != nil {
		return [32]byte{}, err
	}
	if len(b) != 32 {
		return [32]byte{}, fmt.Errorf("expected 32 bytes, got %d", len(b))
	}
	var out [32]byte
	copy(out[:], b)
	return out, nil
}
