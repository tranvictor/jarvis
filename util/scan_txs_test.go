package util

import (
	"reflect"
	"testing"
)

const sampleTxHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const sampleTxHashBare = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

const (
	scanHashA = "0x9c0da020dc5114d12f072c405acb01ef20f77839d1d238842d950409fa51ccf4"
	scanHashB = "0xf046d341fdb329502ffd3d39cec3ab1371b19af6d5b3135550bcee38deed83d9"
	scanHashC = "0x152815b2bd004c24a4ab970f7a5236b19f8c337b24052c49f8e62765ebd16cda"
	scanHashD = "0xdcfebb569af85c6b47a30b49f7f9d42d3825dc3a79dac0aa0630655a4f9d05a8"
)

func TestScanForTxsBareHashDefaultsToMainnet(t *testing.T) {
	nwks, hashes := ScanForTxs(sampleTxHash, "")
	if len(hashes) != 1 || hashes[0] != sampleTxHash {
		t.Fatalf("hashes = %v, want [%s]", hashes, sampleTxHash)
	}
	if nwks[0] != "mainnet" {
		t.Fatalf("network = %q, want mainnet", nwks[0])
	}
}

func TestScanForTxsBareHashUsesDefaultNetwork(t *testing.T) {
	nwks, hashes := ScanForTxs(sampleTxHash, "bsc")
	if len(hashes) != 1 || hashes[0] != sampleTxHash {
		t.Fatalf("hashes = %v, want [%s]", hashes, sampleTxHash)
	}
	if nwks[0] != "bsc" {
		t.Fatalf("network = %q, want bsc", nwks[0])
	}
}

func TestScanForTxsNetworkPrefix(t *testing.T) {
	nwks, hashes := ScanForTxs("mainnet:"+sampleTxHash, "bsc")
	if len(hashes) != 1 || hashes[0] != sampleTxHash {
		t.Fatalf("hashes = %v, want [%s]", hashes, sampleTxHash)
	}
	if nwks[0] != "mainnet" {
		t.Fatalf("network = %q, want mainnet", nwks[0])
	}
}

func TestScanForTxsNetworkPrefixCaseInsensitive(t *testing.T) {
	nwks, hashes := ScanForTxs("MainNet:"+sampleTxHashBare, "")
	if len(hashes) != 1 || hashes[0] != sampleTxHashBare {
		t.Fatalf("hashes = %v, want [%s]", hashes, sampleTxHashBare)
	}
	if nwks[0] != "mainnet" {
		t.Fatalf("network = %q, want mainnet", nwks[0])
	}
}

func TestScanForTxsAliasCanonicalized(t *testing.T) {
	nwks, hashes := ScanForTxs("ethereum:"+sampleTxHash, "bsc")
	if len(hashes) != 1 || hashes[0] != sampleTxHash {
		t.Fatalf("hashes = %v, want [%s]", hashes, sampleTxHash)
	}
	if nwks[0] != "mainnet" {
		t.Fatalf("network = %q, want mainnet", nwks[0])
	}
}

func TestScanForTxsMultiple(t *testing.T) {
	other := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	nwks, hashes := ScanForTxs("mainnet:"+sampleTxHash+" "+other, "")
	if len(hashes) != 2 {
		t.Fatalf("got %d hashes, want 2", len(hashes))
	}
	if hashes[0] != sampleTxHash || hashes[1] != other {
		t.Fatalf("hashes = %v", hashes)
	}
	if nwks[0] != "mainnet" || nwks[1] != "mainnet" {
		t.Fatalf("nwks = %v, want [mainnet mainnet]", nwks)
	}
}

func TestScanForTxsNone(t *testing.T) {
	nwks, hashes := ScanForTxs("not a hash", "mainnet")
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

func TestScanForTxsPastedAmountLines(t *testing.T) {
	// Real paste: amount lines with no network prefix, plus one later
	// hash that already has mainnet:. Every hash must land on mainnet.
	raw := `Transfer back 2,890.418601 ` + scanHashA + `
Transfer back 2,245,177.6751406 ` + scanHashB + `
Transfer back 49,952.530249 ` + scanHashC + `
mainnet:` + scanHashD

	nwks, hashes := ScanForTxs(raw, "")
	wantHashes := []string{scanHashA, scanHashB, scanHashC, scanHashD}
	if !reflect.DeepEqual(hashes, wantHashes) {
		t.Fatalf("hashes = %v, want %v", hashes, wantHashes)
	}
	for i, n := range nwks {
		if n != "mainnet" {
			t.Errorf("network[%d] = %q, want mainnet", i, n)
		}
	}
}

func TestScanForTxsMixedPrefixAndBare(t *testing.T) {
	raw := scanHashA + " mainnet:" + scanHashB + " bsc " + scanHashC + " ethereum:" + scanHashD
	nwks, hashes := ScanForTxs(raw, "")
	wantHashes := []string{scanHashA, scanHashB, scanHashC, scanHashD}
	wantNwks := []string{"mainnet", "mainnet", "bsc", "mainnet"}
	if !reflect.DeepEqual(hashes, wantHashes) {
		t.Fatalf("hashes = %v, want %v", hashes, wantHashes)
	}
	if !reflect.DeepEqual(nwks, wantNwks) {
		t.Fatalf("nwks = %v, want %v", nwks, wantNwks)
	}
}
