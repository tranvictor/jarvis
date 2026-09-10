package cmd

import (
	"reflect"
	"testing"
)

const (
	scanHashA = "0x9c0da020dc5114d12f072c405acb01ef20f77839d1d238842d950409fa51ccf4"
	scanHashB = "0xf046d341fdb329502ffd3d39cec3ab1371b19af6d5b3135550bcee38deed83d9"
	scanHashC = "0x152815b2bd004c24a4ab970f7a5236b19f8c337b24052c49f8e62765ebd16cda"
	scanHashD = "0xdcfebb569af85c6b47a30b49f7f9d42d3825dc3a79dac0aa0630655a4f9d05a8"
)

func TestScanClassicBatchTxsBareHashesDefaultToMainnet(t *testing.T) {
	// Real paste: amount lines with no network prefix, the form that used
	// to print `Classic  :0x…` and skip as "unsupported network". A later
	// `mainnet:`-prefixed hash in the same paste must stay mainnet too.
	raw := `Transfer back 2,890.418601 ` + scanHashA + `
Transfer back 2,245,177.6751406 ` + scanHashB + `
Transfer back 49,952.530249 ` + scanHashC + `
mainnet:` + scanHashD

	nwks, hashes := scanClassicBatchTxs(raw, "")
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

func TestScanClassicBatchTxsMixedPrefixAndBare(t *testing.T) {
	raw := scanHashA + " mainnet:" + scanHashB + " bsc " + scanHashC + " ethereum:" + scanHashD
	nwks, hashes := scanClassicBatchTxs(raw, "")
	wantHashes := []string{scanHashA, scanHashB, scanHashC, scanHashD}
	wantNwks := []string{"mainnet", "mainnet", "bsc", "mainnet"}
	if !reflect.DeepEqual(hashes, wantHashes) {
		t.Fatalf("hashes = %v, want %v", hashes, wantHashes)
	}
	if !reflect.DeepEqual(nwks, wantNwks) {
		t.Fatalf("nwks = %v, want %v", nwks, wantNwks)
	}
}

func TestScanClassicBatchTxsBareHashesUseNetworkFlag(t *testing.T) {
	nwks, hashes := scanClassicBatchTxs(scanHashA+" "+scanHashB, "bsc")
	if !reflect.DeepEqual(hashes, []string{scanHashA, scanHashB}) {
		t.Fatalf("hashes = %v", hashes)
	}
	if !reflect.DeepEqual(nwks, []string{"bsc", "bsc"}) {
		t.Fatalf("nwks = %v, want [bsc bsc]", nwks)
	}
}

func TestScanClassicBatchTxsPrefixedHashBeatsNetworkFlag(t *testing.T) {
	nwks, hashes := scanClassicBatchTxs("mainnet:"+scanHashA, "bsc")
	if !reflect.DeepEqual(hashes, []string{scanHashA}) {
		t.Fatalf("hashes = %v", hashes)
	}
	if !reflect.DeepEqual(nwks, []string{"mainnet"}) {
		t.Fatalf("nwks = %v, want [mainnet]", nwks)
	}
}

func TestScanClassicBatchTxsNone(t *testing.T) {
	nwks, hashes := scanClassicBatchTxs("not a hash", "mainnet")
	if nwks != nil || hashes != nil {
		t.Fatalf("got nwks=%v hashes=%v, want nil", nwks, hashes)
	}
}
