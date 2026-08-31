package util

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const knownSubmissionTopic = "0xc0ba8fe4b176c1714197d43b9cc6bcf797a4a7461c5fe8d0ef6e184ae7601e51"

func TestGnosisMsigSubmissionTopicMatchesABI(t *testing.T) {
	ev, ok := GetGnosisMsigABI().Events["Submission"]
	if !ok {
		t.Fatal("built-in Classic ABI is missing the Submission event")
	}
	if ev.ID != GnosisMsigSubmissionTopic() {
		t.Fatalf("helper %s != ABI event ID %s", GnosisMsigSubmissionTopic().Hex(), ev.ID.Hex())
	}
	want := common.HexToHash(knownSubmissionTopic)
	if ev.ID != want {
		t.Fatalf("Submission topic = %s, want %s (on-chain Classic event)", ev.ID.Hex(), want.Hex())
	}
}

func TestGnosisMsigTxIDFromLogs(t *testing.T) {
	msig := "0x1111111111111111111111111111111111111111"
	other := "0x2222222222222222222222222222222222222222"
	txid := common.BigToHash(big.NewInt(7))

	logs := []*types.Log{
		{Address: common.HexToAddress(other), Topics: []common.Hash{GnosisMsigSubmissionTopic(), txid}},
		{Address: common.HexToAddress(msig), Topics: []common.Hash{common.HexToHash("0x01"), txid}},
		{Address: common.HexToAddress(msig), Topics: []common.Hash{GnosisMsigSubmissionTopic(), txid}},
	}

	got := GnosisMsigTxIDFromLogs(logs, msig)
	if got == nil || got.Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("got %v, want 7", got)
	}
	if GnosisMsigTxIDFromLogs(logs[:2], msig) != nil {
		t.Fatal("expected nil when no matching Submission log")
	}
}

func TestGnosisMsigTxIDFromConfirmationLog(t *testing.T) {
	// confirmTransaction emits Confirmation(sender, transactionId), not Submission.
	// Polygon tx 0x76766bc8… is this shape: users paste the approve hash, not the init hash.
	msig := "0x1b0868fd8a174e979135812db866e5eaed4b3357"
	sender := common.HexToAddress("0xc18ebcdbcffad08ce433dd10bdef02424bd24e5a")
	confirm := GetGnosisMsigABI().Events["Confirmation"]
	logs := []*types.Log{{
		Address: common.HexToAddress(msig),
		Topics: []common.Hash{
			confirm.ID,
			common.BytesToHash(sender.Bytes()),
			common.BigToHash(big.NewInt(414)),
		},
	}}
	got := GnosisMsigTxIDFromLogs(logs, msig)
	if got == nil || got.Cmp(big.NewInt(414)) != 0 {
		t.Fatalf("got %v, want 414", got)
	}
}

func TestGnosisMsigTxIDFromCalldataConfirm(t *testing.T) {
	data, err := GetGnosisMsigABI().Pack("confirmTransaction", big.NewInt(414))
	if err != nil {
		t.Fatal(err)
	}
	got := GnosisMsigTxIDFromCalldata(data)
	if got == nil || got.Cmp(big.NewInt(414)) != 0 {
		t.Fatalf("got %v, want 414", got)
	}
	if GnosisMsigTxIDFromCalldata([]byte{0x01, 0x02, 0x03, 0x04}) != nil {
		t.Fatal("expected nil for unknown selector")
	}
}
