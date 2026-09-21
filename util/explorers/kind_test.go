package explorers

import (
	"strings"
	"testing"
)

func TestInferKind(t *testing.T) {
	cases := []struct {
		url  string
		want Kind
	}{
		{"https://api.etherscan.io/v2", KindEtherscan},
		{"https://api.etherscan.io/v2/api", KindEtherscan},
		{"https://api.etherscan.io/api", KindEtherscan},
		{"https://api.bscscan.com/api", KindEtherscan},
		{"https://robin.etherscan.io/api", KindEtherscan},
		{"https://api.etherscan.io/v2?chainid=4663", KindEtherscan},
		{"https://api.routescan.io/v2/network/mainnet/evm/43114/etherscan/", KindRoutescan},
		{"https://api.snowtrace.io/api", KindRoutescan},
		{"https://eth.blockscout.com/api", KindBlockscout},
		{"https://eth.blockscout.com/api/v2", KindBlockscout},
		{"https://bitfi-ledger-testnet-explorer.alt.technology/api/v2", KindBlockscout},
		{"https://api.robinscan.io", KindRobinscan},
		{"https://robinscan.io/api", KindRobinscan},
		{"", KindEtherscan},
	}
	for _, tc := range cases {
		if got := InferKind(tc.url); got != tc.want {
			t.Errorf("InferKind(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestParseKind(t *testing.T) {
	k, err := ParseKind(" Blockscout ")
	if err != nil || k != KindBlockscout {
		t.Fatalf("ParseKind blockscout: %q %v", k, err)
	}
	k, err = ParseKind("")
	if err != nil || k != "" {
		t.Fatalf("empty kind is infer-later: %q %v", k, err)
	}
	if _, err := ParseKind("sourcify"); err == nil {
		t.Fatal("Sourcify is an overlay, not a Kind")
	}
}

func TestExplorerClassURLs(t *testing.T) {
	addr := "0xCC000E5aC85e139b15C96633DCe2A041202946BF"

	t.Run("etherscan v2 only chainid getsourcecode", func(t *testing.T) {
		ee := New(KindEtherscan, "https://api.etherscan.io/v2", "k", 1)
		urls := ee.contractInfoURLs(addr)
		if len(urls) != 1 {
			t.Fatalf("v2 URLs: %v", urls)
		}
		if !strings.Contains(urls[0], "chainid=1") || !strings.Contains(urls[0], "action=getsourcecode") {
			t.Fatalf("v2 URL: %s", urls[0])
		}
		assertNoOtherFamilyPaths(t, urls)
	})

	t.Run("etherscan v1 tries no-chainid then chainid", func(t *testing.T) {
		ee := New(KindEtherscan, "https://api.bscscan.com/api", "k", 56)
		urls := ee.contractInfoURLs(addr)
		if len(urls) != 2 {
			t.Fatalf("v1 URLs: %v", urls)
		}
		if strings.Contains(urls[0], "chainid=") {
			t.Fatalf("v1 should try no-chainid first: %s", urls[0])
		}
		if !strings.Contains(urls[1], "chainid=56") {
			t.Fatalf("v1 fallback should send chainid: %s", urls[1])
		}
		assertNoOtherFamilyPaths(t, urls)
	})

	t.Run("routescan prefers path-embedded chain, no REST shotgun", func(t *testing.T) {
		ee := New(KindRoutescan, "https://api.routescan.io/v2/network/mainnet/evm/43114/etherscan/", "k", 43114)
		urls := ee.contractInfoURLs(addr)
		if len(urls) == 0 || strings.Contains(urls[0], "chainid=") {
			t.Fatalf("routescan should prefer no-chainid (chain is in the path): %v", urls)
		}
		assertNoOtherFamilyPaths(t, urls)
	})

	t.Run("robinscan only /api/contracts", func(t *testing.T) {
		ee := New(KindRobinscan, "https://robinscan.io", "", 4663)
		urls := ee.contractInfoURLs(addr)
		if len(urls) != 1 || urls[0] != "https://robinscan.io/api/contracts/"+addr {
			t.Fatalf("robinscan URLs: %v", urls)
		}
	})

	t.Run("blockscout REST plural then etherscan-compat", func(t *testing.T) {
		ee := New(KindBlockscout, "https://eth.blockscout.com", "", 1)
		urls := ee.contractInfoURLs(addr)
		if len(urls) < 2 || urls[0] != "https://eth.blockscout.com/api/v2/smart-contracts/"+addr {
			t.Fatalf("blockscout URLs: %v", urls)
		}
		joined := strings.Join(urls, "\n")
		if strings.Contains(joined, "/api/contracts/") {
			t.Fatalf("blockscout must not hit Robinscan paths: %v", urls)
		}
		if strings.Contains(joined, "/smart-contract/"+addr) && !strings.Contains(joined, "/smart-contracts/"+addr) {
			t.Fatalf("bare blockscout host must not use HTML /smart-contract/: %v", urls)
		}
	})

	t.Run("blockscout domain already /api/v2 also tries singular", func(t *testing.T) {
		ee := New(KindBlockscout, "https://bitfi.example/api/v2", "", 891891)
		urls := ee.contractInfoURLs(addr)
		wantPlural := "https://bitfi.example/api/v2/smart-contracts/" + addr
		wantSingular := "https://bitfi.example/api/v2/smart-contract/" + addr
		if urls[0] != wantPlural {
			t.Fatalf("plural first: %v", urls)
		}
		found := false
		for _, u := range urls {
			if u == wantSingular {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing Bitfi singular REST: %v", urls)
		}
	})
}

func assertNoOtherFamilyPaths(t *testing.T, urls []string) {
	t.Helper()
	for _, u := range urls {
		if strings.Contains(u, "/api/contracts/") || strings.Contains(u, "/smart-contracts/") || strings.Contains(u, "/smart-contract/") {
			t.Fatalf("class URL leaked another family's REST path: %s", u)
		}
	}
}
