package util

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/util/cache"
)

func TestAddressesFromTxInfoCollectsUniqueParticipants(t *testing.T) {
	from := common.HexToAddress("0x1111111111111111111111111111111111111111")
	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	logA := common.HexToAddress("0x3333333333333333333333333333333333333333")
	inner := types.NewTx(&types.LegacyTx{To: &to, Gas: 21000})
	tx := &jarviscommon.Transaction{
		Transaction: inner,
		Extra:       jarviscommon.TxExtraInfo{From: &from},
	}
	receipt := &types.Receipt{
		Logs: []*types.Log{
			{Address: logA},
			{Address: to},
		},
	}

	got := addressesFromTxInfo(&jarviscommon.TxInfo{Tx: tx, Receipt: receipt})
	want := map[string]bool{
		from.Hex(): true,
		to.Hex():   true,
		logA.Hex(): true,
	}
	if len(got) != 3 {
		t.Fatalf("got %d addresses: %v", len(got), got)
	}
	for _, addr := range got {
		if !want[addr] {
			t.Fatalf("unexpected %s in %v", addr, got)
		}
	}
}

func TestAddressesFromTxInfoNilSafe(t *testing.T) {
	if addrs := addressesFromTxInfo(nil); len(addrs) != 0 {
		t.Fatalf("nil txinfo: %v", addrs)
	}
	if addrs := addressesFromTxInfo(&jarviscommon.TxInfo{}); len(addrs) != 0 {
		t.Fatalf("empty txinfo: %v", addrs)
	}
}

func TestWarmAddressesSkipsEmptyAndZero(t *testing.T) {
	// Must not panic or hit the network for sentinels.
	WarmAddresses([]string{"", "0x0000000000000000000000000000000000000000", "  "}, nil)
}

func TestCacheExplorerABI(t *testing.T) {
	dir := t.TempDir()
	cache.ResetForTest(filepath.Join(dir, "cache.json"))
	cacheExplorerABI("0xabc", `[{"type":"function","name":"foo"}]`)
	got, found := cache.GetCache("0xabc_abi")
	if !found || !strings.Contains(got, "foo") {
		t.Fatalf("cached ABI: %q %v", got, found)
	}
}
