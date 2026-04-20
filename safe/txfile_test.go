package safe

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestTxFileRoundtrip verifies WriteTxFile + ReadTxFile + ToPending
// preserves every field we care about, including the full 65-byte sig
// and the very-large uint256 we've seen in real withdrawals that used
// to break string/number JSON handling.
func TestTxFileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proposal.json")

	to := common.HexToAddress("0x87870Bca3F3fD6335C3F4ce8392D69350B4fA4E2")
	// uint256.max — mirrors the aave-v3 withdraw "take everything" value
	// that originally revealed the JSON numeric-field bug.
	maxU256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	data, _ := hex.DecodeString("69328dec000000000000000000000000a0b86991c6218b36c1d19d4a2e9eb0ce3606eb48ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff00000000000000000000000071f8f067348d47cced223ea24d2d77235bea722b")

	tx := &SafeTx{
		To:             to,
		Value:          big.NewInt(0),
		Data:           data,
		Operation:      OpCall,
		SafeTxGas:      big.NewInt(0),
		BaseGas:        big.NewInt(0),
		GasPrice:       big.NewInt(0),
		GasToken:       common.Address{},
		RefundReceiver: common.Address{},
		Nonce:          big.NewInt(22),
	}
	_ = maxU256 // referenced for documentation of intent; ensure it can round-trip through bigStr
	// substitute Value so we also verify large-uint round-trip
	tx.Value = maxU256

	safeAddr := "0x71f8f067348d47cced223eA24D2D77235bea722B"
	chainID := uint64(1)

	var hash [32]byte
	copy(hash[:], common.HexToHash("0x82c28e25b40c865440a0e89fd9578fe62f629f5fafaa0af0587342f7b4b41efe").Bytes())

	owner := common.HexToAddress("0xd3dd02ef3E32baF7b69edDec34F0444F08Fedb65")
	rawSig, _ := hex.DecodeString("e44775d7acdae69558285cf246400b6f9fe2b08b786064a8bcf7f5d7e81babe321d81358a46e64874dfca8f0698371197fdc841e57c41ac551b84282fd4fcda91b")
	sigs := []OwnerSig{
		{Owner: owner, Sig: rawSig},
		OnChainApprovalSig(common.HexToAddress("0xA4FDdCFa01159D984Ae031E46a68856842B58Fa4")),
	}

	if err := WriteTxFile(path, safeAddr, chainID, tx, hash, sigs); err != nil {
		t.Fatalf("WriteTxFile: %v", err)
	}

	tf, err := ReadTxFile(path)
	if err != nil {
		t.Fatalf("ReadTxFile: %v", err)
	}
	if tf.ChainID != chainID {
		t.Fatalf("chain id mismatch: got %d want %d", tf.ChainID, chainID)
	}
	if !common.IsHexAddress(tf.Safe) || common.HexToAddress(tf.Safe) != common.HexToAddress(safeAddr) {
		t.Fatalf("safe mismatch: %s", tf.Safe)
	}

	pending, err := tf.ToPending()
	if err != nil {
		t.Fatalf("ToPending: %v", err)
	}
	if pending.SafeTx.Value.Cmp(maxU256) != 0 {
		t.Fatalf("value didn't round-trip: got %s", pending.SafeTx.Value.String())
	}
	if !bytes.Equal(pending.SafeTx.Data, data) {
		t.Fatalf("data didn't round-trip")
	}
	if pending.SafeTx.Nonce.Cmp(big.NewInt(22)) != 0 {
		t.Fatalf("nonce didn't round-trip: got %s", pending.SafeTx.Nonce.String())
	}
	if len(pending.Sigs) != 2 {
		t.Fatalf("expected 2 sigs, got %d", len(pending.Sigs))
	}
	if !bytes.Equal(pending.Sigs[0].Sig, rawSig) {
		t.Fatalf("ECDSA sig didn't round-trip")
	}
	if !IsOnChainApproval(pending.Sigs[1].Sig) {
		t.Fatalf("on-chain approval marker didn't round-trip: %x", pending.Sigs[1].Sig)
	}
	if pending.Sigs[1].Owner != common.HexToAddress("0xA4FDdCFa01159D984Ae031E46a68856842B58Fa4") {
		t.Fatalf("on-chain approval owner mismatch: %s", pending.Sigs[1].Owner.Hex())
	}
}

// TestTxFileChainIDRejected guards against silently loading a file that
// was built for a different chain (which would otherwise produce the
// wrong on-chain execution).
func TestTxFileHasSigForOwner(t *testing.T) {
	tf := TxFile{
		Sigs: []TxFileSignature{
			{Owner: "0xd3dd02ef3E32baF7b69edDec34F0444F08Fedb65"},
		},
	}
	if !tf.HasSigForOwner(common.HexToAddress("0xD3DD02EF3E32BAF7B69EDDEC34F0444F08FEDB65")) {
		t.Fatalf("HasSigForOwner should match case-insensitively")
	}
	if tf.HasSigForOwner(common.HexToAddress("0xA4FDdCFa01159D984Ae031E46a68856842B58Fa4")) {
		t.Fatalf("HasSigForOwner false positive")
	}
}
