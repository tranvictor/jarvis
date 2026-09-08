package explorers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApiEndpointDoesNotDoubleAPI(t *testing.T) {
	ee := NewEtherscanLikeExplorer("https://robinscan.io/api", "", 4663)
	if got := ee.apiEndpoint(); got != "https://robinscan.io/api" {
		t.Fatalf("apiEndpoint = %q", got)
	}
	if got := ee.origin(); got != "https://robinscan.io" {
		t.Fatalf("origin = %q", got)
	}
	ee = NewEtherscanLikeExplorer("https://api.etherscan.io/v2", "", 1)
	if got := ee.apiEndpoint(); got != "https://api.etherscan.io/v2/api" {
		t.Fatalf("etherscan v2 apiEndpoint = %q", got)
	}
}

func TestGetABIStringFallsBackToJSONContractEndpoint(t *testing.T) {
	const addr = "0x0Bd7D308f8E1639FAb988df18A8011f41EAcAD73"
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"not found"}`)
	})
	mux.HandleFunc("/api/contracts/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(strings.ToLower(r.URL.Path), strings.ToLower(addr)) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{
			"address":"0x0bd7d308f8e1639fab988df18a8011f41eacad73",
			"isVerified":true,
			"name":"TransparentUpgradeableProxy",
			"proxyType":"eip1967",
			"implementation":"0xc6b81b429797e0f555440b70cd99e032d7ae947e",
			"abi":[{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"implementation","type":"address"}],"name":"Upgraded","type":"event"}]
		}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ee := NewEtherscanLikeExplorer(srv.URL, "SECRETKEY", 4663)
	got, err := ee.GetABIString(addr)
	if err != nil {
		t.Fatalf("GetABIString: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(got), &arr); err != nil {
		t.Fatalf("ABI is not JSON: %q %v", got, err)
	}
	if len(arr) != 1 || arr[0]["name"] != "Upgraded" {
		t.Fatalf("unexpected ABI %s", got)
	}

	info, err := ee.GetContractInfo(addr)
	if err != nil {
		t.Fatalf("GetContractInfo: %v", err)
	}
	if !info.IsVerified || !info.IsProxy {
		t.Fatalf("verified proxy: %+v", info)
	}
	if !strings.EqualFold(info.Implementation, "0xc6b81b429797e0f555440b70cd99e032d7ae947e") {
		t.Fatalf("implementation = %q", info.Implementation)
	}
	if info.Name != "TransparentUpgradeableProxy" {
		t.Fatalf("name = %q", info.Name)
	}
}

func TestGetABIStringFallsBackToBlockscoutV2(t *testing.T) {
	const addr = "0x0000000000000000000000000000000000000001"
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/contracts/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v2/smart-contracts/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{
			"name":"WETH",
			"is_verified":true,
			"is_proxy":true,
			"implementations":[{"address":"0x00000000000000000000000000000000000000aa"}],
			"abi":[{"type":"function","name":"deposit","inputs":[],"outputs":[],"stateMutability":"payable"}]
		}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ee := NewEtherscanLikeExplorer(srv.URL, "", 1)
	got, err := ee.GetABIString(addr)
	if err != nil || !strings.Contains(got, "deposit") {
		t.Fatalf("GetABIString: %q %v", got, err)
	}
	info, err := ee.GetContractInfo(addr)
	if err != nil {
		t.Fatalf("GetContractInfo: %v", err)
	}
	if !info.IsProxy || !strings.EqualFold(info.Implementation, "0x00000000000000000000000000000000000000aa") {
		t.Fatalf("info: %+v", info)
	}
}

func TestGetContractInfoStillTreatsEtherscanUnverifiedAsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":"0","message":"NOTOK","result":"Contract source code not verified"}`)
	}))
	defer srv.Close()

	ee := NewEtherscanLikeExplorer(srv.URL, "SECRETKEY", 1)
	info, err := ee.GetContractInfo("0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if info != (ContractInfo{}) {
		t.Fatalf("unverified must be empty, got %+v", info)
	}
}
