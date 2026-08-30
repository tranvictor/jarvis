package util

import (
	"testing"
)

const sampleTxHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const sampleTxHashBare = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestScanForTxsBareHash(t *testing.T) {
	nwks, hashes := ScanForTxs(sampleTxHash)
	if len(hashes) != 1 || hashes[0] != sampleTxHash {
		t.Fatalf("hashes = %v, want [%s]", hashes, sampleTxHash)
	}
	if nwks[0] != "" {
		t.Fatalf("network = %q, want empty", nwks[0])
	}
}

func TestScanForTxsNetworkPrefix(t *testing.T) {
	nwks, hashes := ScanForTxs("mainnet:" + sampleTxHash)
	if len(hashes) != 1 || hashes[0] != sampleTxHash {
		t.Fatalf("hashes = %v, want [%s]", hashes, sampleTxHash)
	}
	if nwks[0] != "mainnet" {
		t.Fatalf("network = %q, want mainnet", nwks[0])
	}
}

func TestScanForTxsNetworkPrefixCaseInsensitive(t *testing.T) {
	nwks, hashes := ScanForTxs("MainNet:" + sampleTxHashBare)
	if len(hashes) != 1 || hashes[0] != sampleTxHashBare {
		t.Fatalf("hashes = %v, want [%s]", hashes, sampleTxHashBare)
	}
	if nwks[0] != "mainnet" {
		t.Fatalf("network = %q, want mainnet", nwks[0])
	}
}

func TestScanForTxsMultiple(t *testing.T) {
	other := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	nwks, hashes := ScanForTxs("mainnet:" + sampleTxHash + " " + other)
	if len(hashes) != 2 {
		t.Fatalf("got %d hashes, want 2", len(hashes))
	}
	if hashes[0] != sampleTxHash || hashes[1] != other {
		t.Fatalf("hashes = %v", hashes)
	}
	if nwks[0] != "mainnet" || nwks[1] != "" {
		t.Fatalf("nwks = %v", nwks)
	}
}

func TestScanForTxsNone(t *testing.T) {
	nwks, hashes := ScanForTxs("not a hash")
	if len(nwks) != 0 || len(hashes) != 0 {
		t.Fatalf("got nwks=%v hashes=%v, want empty", nwks, hashes)
	}
}

func TestScanForTxHashes(t *testing.T) {
	got := ScanForTxHashes("mainnet:" + sampleTxHash)
	if len(got) != 1 || got[0] != sampleTxHash {
		t.Fatalf("ScanForTxHashes = %v, want [%s]", got, sampleTxHash)
	}
	if empty := ScanForTxHashes(""); len(empty) != 0 {
		t.Fatalf("empty input = %v, want []", empty)
	}
}
