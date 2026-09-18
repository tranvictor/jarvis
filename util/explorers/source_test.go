package explorers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFlattenSourceJSONInput(t *testing.T) {
	raw := `{{"language":"Solidity","sources":{"A.sol":{"content":"pragma solidity ^0.8;"}}}}`
	got := flattenSource(raw)
	if !strings.Contains(got, "file: A.sol") || !strings.Contains(got, "pragma solidity") {
		t.Fatalf("flatten: %q", got)
	}
}

func TestGetVerifiedSourceEtherscan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"1","message":"OK","result":[{"ContractName":"Foo","ABI":"[]","Proxy":"0","Implementation":"","SourceCode":"contract Foo {}"}]}`))
	}))
	t.Cleanup(srv.Close)
	ee := NewEtherscanLikeExplorer(srv.URL, "k", 1)
	src, err := ee.GetVerifiedSource("0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if !src.Verified || !strings.Contains(src.Source, "contract Foo") {
		t.Fatalf("%+v", src)
	}
}

func TestGetVerifiedSourceRobinscanSourceFiles(t *testing.T) {
	const impl = "0x68184C449E1a8f34fA18d289737129FD27B66f8F"
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/contracts/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"address":"0x68184C449E1a8f34fA18d289737129FD27B66f8F",
			"isVerified":true,
			"name":"USDG",
			"sourceFiles":[
				{"path":"contracts/stablecoins/USDG.sol","content":"// SPDX-License-Identifier: MIT\npragma solidity 0.8.28;\ncontract USDG {}"},
				{"path":"contracts/lib/Initializable.sol","content":"contract Initializable {}"}
			],
			"abi":[{"type":"function","name":"approve","inputs":[{"name":"spender","type":"address"},{"name":"value","type":"uint256"}],"outputs":[{"type":"bool"}]}]
		}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ee := NewEtherscanLikeExplorer(srv.URL, "", 1)
	src, err := ee.GetVerifiedSource(impl)
	if err != nil {
		t.Fatal(err)
	}
	if !src.Verified {
		t.Fatalf("Robinscan isVerified+sourceFiles must count as verified: %+v", src)
	}
	if !strings.Contains(src.Source, "file: contracts/stablecoins/USDG.sol") || !strings.Contains(src.Source, "contract USDG") {
		t.Fatalf("sourceFiles not flattened: %q", src.Source)
	}
}

func TestIsRateLimitedIgnoresLargeSourceBodies(t *testing.T) {
	small := []byte(`{"status":"0","result":"Max calls per sec rate limit reached (3/sec)"}`)
	if !isRateLimited(small) {
		t.Fatal("short explorer error must still count as rate limited")
	}
	big := make([]byte, 5000)
	copy(big, []byte(`{"sourceFiles":[{"content":"/// rate limit comments in Solidity"}]}`))
	if isRateLimited(big) {
		t.Fatal("verified source mentioning rate limits must not be treated as HTTP rate limiting")
	}
}
