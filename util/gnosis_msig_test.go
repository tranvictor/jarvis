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
