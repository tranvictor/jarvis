package util

import (
	"testing"

	"github.com/tranvictor/jarvis/config"
)

func TestScanForTxsBareHashInheritsConfigNetwork(t *testing.T) {
	const hash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	old := config.NetworkString
	t.Cleanup(func() { config.NetworkString = old })

	config.NetworkString = "bsc"
	nwks, hashes := ScanForTxs(hash)
	if len(hashes) != 1 || hashes[0] != hash {
		t.Fatalf("hashes = %v, want [%s]", hashes, hash)
	}
	if nwks[0] != "bsc" {
		t.Fatalf("network = %q, want bsc (from -k/--network)", nwks[0])
	}

	config.NetworkString = ""
	nwks, _ = ScanForTxs(hash)
	if nwks[0] != "mainnet" {
		t.Fatalf("empty config.NetworkString: network = %q, want mainnet", nwks[0])
	}
}

func TestScanForTxsPrefixBeatsConfigNetwork(t *testing.T) {
	const hash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	old := config.NetworkString
	t.Cleanup(func() { config.NetworkString = old })

	config.NetworkString = "bsc"
	nwks, _ := ScanForTxs("ethereum:" + hash)
	if nwks[0] != "mainnet" {
		t.Fatalf("network = %q, want mainnet (ethereum alias)", nwks[0])
	}
}
