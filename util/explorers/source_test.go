package explorers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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
	sourcify404(t)
	const impl = "0x68184C449E1a8f34fA18d289737129FD27B66f8F"
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Robinscan kind must not hit Etherscan module=contract: %s", r.URL)
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

	ee := New(KindRobinscan, srv.URL, "", 4663)
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

func sourcify404(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"match":null}`))
	}))
	t.Cleanup(srv.Close)
	orig := sourcifyContractURL
	sourcifyContractURL = func(uint64, string) string { return srv.URL }
	t.Cleanup(func() { sourcifyContractURL = orig })
}

func TestGetVerifiedSourceFollowsEtherscanSimilarMatch(t *testing.T) {
	sourcify404(t)
	const (
		dest = "0xCC000E5aC85e139b15C96633DCe2A041202946BF"
		twin = "0xE6A7338cba0a1070adfb22c07115299605454713"
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		switch strings.ToLower(r.URL.Query().Get("address")) {
		case strings.ToLower(dest):
			w.Write([]byte(`{"status":"1","message":"OK","result":[{"SourceCode":"","ABI":"Contract source code not verified","ContractName":"","CompilerVersion":"","Proxy":"0","Implementation":"","SimilarMatch":"` + twin + `"}]}`))
		case strings.ToLower(twin):
			w.Write([]byte(`{"status":"1","message":"OK","result":[{"SourceCode":"contract MultiSigWalletWithDailyLimit {}","ABI":"[]","ContractName":"MultiSigWalletWithDailyLimit","Proxy":"0","Implementation":"","SimilarMatch":""}]}`))
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/api/contracts/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("etherscan kind must not hit Robinscan: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v2/smart-contracts/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("etherscan kind must not hit Blockscout REST: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ee := NewEtherscanLikeExplorer(srv.URL, "k", 1)
	src, err := ee.GetVerifiedSource(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !src.Verified || !strings.Contains(src.Source, "contract MultiSigWalletWithDailyLimit") {
		t.Fatalf("similar-match twin source must count as verified: %+v", src)
	}
	if !strings.EqualFold(src.Address, dest) {
		t.Fatalf("address should stay the queried contract, got %s", src.Address)
	}
}

func TestGetVerifiedSourceFallsBackToSourcify(t *testing.T) {
	srcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"match":"match","sources":{"MultiSigWalletWithDailyLimit.sol":{"content":"contract Factory {}"}}}`))
	}))
	t.Cleanup(srcSrv.Close)
	orig := sourcifyContractURL
	sourcifyContractURL = func(uint64, string) string { return srcSrv.URL }
	t.Cleanup(func() { sourcifyContractURL = orig })

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"1","message":"OK","result":[{"SourceCode":"","ABI":"Contract source code not verified","SimilarMatch":""}]}`))
	})
	mux.HandleFunc("/api/contracts/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("etherscan kind must not hit Robinscan: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v2/smart-contracts/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("etherscan kind must not hit Blockscout REST: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ee := NewEtherscanLikeExplorer(srv.URL, "k", 1)
	src, err := ee.GetVerifiedSource("0xCC000E5aC85e139b15C96633DCe2A041202946BF")
	if err != nil {
		t.Fatal(err)
	}
	if !src.Verified || !strings.Contains(src.Source, "contract Factory") {
		t.Fatalf("Sourcify exact match must count as verified: %+v", src)
	}
}

func TestGetVerifiedSourceBlockscoutAdditionalSources(t *testing.T) {
	sourcify404(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/contracts/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Blockscout kind must not hit Robinscan: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v2/smart-contracts/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"name":"Token",
			"is_verified":true,
			"is_partially_verified":true,
			"file_path":"Token.sol",
			"source_code":"pragma solidity ^0.8.0; contract Token {}",
			"additional_sources":[{"file_path":"lib/Safe.sol","source_code":"contract Safe {}"}],
			"abi":[{"type":"function","name":"transfer","inputs":[],"outputs":[]}]
		}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ee := New(KindBlockscout, srv.URL, "", 1)
	src, err := ee.GetVerifiedSource("0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if !src.Verified {
		t.Fatalf("Blockscout v2 verified source: %+v", src)
	}
	if !strings.Contains(src.Source, "file: Token.sol") || !strings.Contains(src.Source, "file: lib/Safe.sol") {
		t.Fatalf("additional_sources not flattened: %q", src.Source)
	}
}

func TestGetVerifiedSourceAPIErrorIsNotUnverified(t *testing.T) {
	sourcify404(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"0","message":"NOTOK","result":"Missing chainid parameter (required for v2 api)"}`))
	})
	mux.HandleFunc("/api/contracts/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("etherscan kind must not hit Robinscan: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v2/smart-contracts/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("etherscan kind must not fall through to Blockscout REST: %s", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"Foo","is_verified":true,"source_code":"contract Foo {}","abi":[]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ee := New(KindEtherscan, srv.URL, "k", 1)
	src, err := ee.GetVerifiedSource("0x0000000000000000000000000000000000000001")
	if err == nil {
		t.Fatalf("missing-chainid is an API error, not unverified, and must not use another family's REST: %+v", src)
	}
	if src.Verified {
		t.Fatalf("must not mark verified: %+v", src)
	}
}

func TestGetVerifiedSourceRateLimitIsFetchError(t *testing.T) {
	sourcify404(t)
	prev := abiRetryDelay
	abiRetryDelay = 0
	t.Cleanup(func() { abiRetryDelay = prev })

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"0","message":"NOTOK","result":"Max calls per sec rate limit reached (3/sec)"}`))
	})
	mux.HandleFunc("/api/contracts/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("etherscan kind must not hit Robinscan: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v2/smart-contracts/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("etherscan kind must not hit Blockscout REST: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ee := NewEtherscanLikeExplorer(srv.URL, "k", 1)
	src, err := ee.GetVerifiedSource("0x0000000000000000000000000000000000000001")
	if err == nil {
		t.Fatalf("rate limit with no verified fallback must be an error, got %+v", src)
	}
	if src.Verified {
		t.Fatalf("must not mark verified: %+v", src)
	}
}

func TestParseEtherscanSourceBlockscoutObjectResult(t *testing.T) {
	body := []byte(`{"status":"1","message":"OK","result":{"SourceCode":"contract Foo {}","ABI":"[]","ContractName":"Foo","IsProxy":"false"}}`)
	src, ok := parseEtherscanSource(body, "0xabc")
	if !ok || !src.Verified || !strings.Contains(src.Source, "contract Foo") {
		t.Fatalf("%v %+v", ok, src)
	}
}

func TestParseSourcifySourcesRejectsEmptyMatch(t *testing.T) {
	if parseSourcifySources([]byte(`{"match":null,"sources":{}}`)) != "" {
		t.Fatal("empty sourcify payload must not look verified")
	}
}

func TestGetVerifiedSourceBlockscoutSingularSmartContract(t *testing.T) {
	sourcify404(t)
	const addr = "0x0000000000000000000000000000000000000001"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/smart-contracts/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v2/smart-contract/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"name":"RollupToken",
			"is_verified":true,
			"file_path":"Token.sol",
			"source_code":"contract RollupToken {}",
			"additional_sources":[{"file_path":"lib/Safe.sol","source_code":"contract Safe {}"}],
			"abi":[]
		}`))
	})
	mux.HandleFunc("/api/contracts/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Blockscout kind must not hit Robinscan: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ee := New(KindBlockscout, srv.URL+"/api/v2", "", 891891)
	if ee.Kind != KindBlockscout {
		t.Fatalf("Kind = %q", ee.Kind)
	}
	src, err := ee.GetVerifiedSource(addr)
	if err != nil {
		t.Fatal(err)
	}
	if !src.Verified || !strings.Contains(src.Source, "contract RollupToken") || !strings.Contains(src.Source, "file: lib/Safe.sol") {
		t.Fatalf("Bitfi-style singular REST: %+v", src)
	}
}

func TestGetVerifiedSourceStopsAfterUnverifiedParse(t *testing.T) {
	sourcify404(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(`{"status":"1","message":"OK","result":[{"SourceCode":"","ABI":"Contract source code not verified","SimilarMatch":""}]}`))
	}))
	t.Cleanup(srv.Close)

	ee := NewEtherscanLikeExplorer(srv.URL, "k", 56)
	addr := "0x00000000000000000000000000000000000000aa"
	if n := len(ee.contractInfoURLs(addr)); n != 2 {
		t.Fatalf("v1 family must try two URL shapes, got %d: %v", n, ee.contractInfoURLs(addr))
	}
	src, err := ee.GetVerifiedSource(addr)
	if err != nil {
		t.Fatal(err)
	}
	if src.Verified {
		t.Fatalf("unverified: %+v", src)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("unverified parse must not hit remaining family URLs, got %d calls", got)
	}
}

func TestGetVerifiedSourceAndContractInfoShareHTTP(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(`{"status":"1","message":"OK","result":[{"ContractName":"Foo","ABI":"[]","Proxy":"0","Implementation":"","SourceCode":"contract Foo {}"}]}`))
	}))
	t.Cleanup(srv.Close)

	ee := NewEtherscanLikeExplorer(srv.URL, "k", 1)
	addr := "0x00000000000000000000000000000000000000bb"
	src, err := ee.GetVerifiedSource(addr)
	if err != nil || !src.Verified || !strings.Contains(src.Source, "contract Foo") {
		t.Fatalf("%+v %v", src, err)
	}
	info, err := ee.GetContractInfo(addr)
	if err != nil || info.Name != "Foo" || !info.IsVerified {
		t.Fatalf("%+v %v", info, err)
	}
	src2, err := ee.GetVerifiedSource(addr)
	if err != nil || !src2.Verified {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("source + info + repeat source must share one HTTP GET, got %d", got)
	}
}
