package erc7730

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestKyberMetaAggregationRouterMatch(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".tmp-registry",
		"clear-signing-erc7730-registry-master", "registry", "kyberswap",
		"calldata-MetaAggregationRouterV2.json"))
	if err != nil {
		t.Skip("registry tarball not present locally:", err)
	}
	d, err := ParseDescriptor(raw)
	if err != nil {
		t.Fatalf("parse descriptor: %v", err)
	}

	const formatKey = "swap((address callTarget, address approveTarget, bytes targetData, (address srcToken, address dstToken, address[] srcReceivers, uint256[] srcAmounts, address[] feeReceivers, uint256[] feeAmounts, address dstReceiver, uint256 amount, uint256 minReturnAmount, uint256 flags, bytes permit) desc, bytes clientData) execution)"
	parsed, err := parseFormatKey(formatKey)
	if err != nil {
		t.Fatalf("parse format key: %v", err)
	}
	wantSel := "e21fd0e9"
	gotSel := hex.EncodeToString(parsed.Selector)
	if gotSel != wantSel {
		t.Fatalf("selector mismatch: got %s want %s", gotSel, wantSel)
	}

	calldata, _ := hex.DecodeString(wantSel + "0000000000000000000000000000000000000000000000000000000000000020")
	src := staticSource{d}
	m := FindContractMatch(src, ContractMatchInput{
		ChainID:  56,
		To:       "0x6131B5fae19EA4f9D964eAc0408E4408b66337b5",
		Calldata: calldata,
	})
	if m == nil {
		t.Fatal("expected contract match for Kyber swap selector on BSC")
	}
}

func TestKyberDescriptorLoadsFromRegistryTree(t *testing.T) {
	srcRoot := filepath.Join("..", "..", ".tmp-registry",
		"clear-signing-erc7730-registry-master", "registry")
	srcFile := filepath.Join(srcRoot, "kyberswap", "calldata-MetaAggregationRouterV2.json")
	if _, err := os.Stat(srcFile); err != nil {
		t.Skip("registry tarball not present locally:", err)
	}
	tmp := t.TempDir()
	regDir := filepath.Join(tmp, "registry", "kyberswap")
	if err := os.MkdirAll(regDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(srcFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(regDir, "calldata-MetaAggregationRouterV2.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewLocalRegistry(tmp)
	ds := r.FindByContract(56, "0x6131B5fae19EA4f9D964eAc0408E4408b66337b5")
	if len(ds) == 0 {
		t.Fatal("registry index missed Kyber descriptor on BSC")
	}
}
