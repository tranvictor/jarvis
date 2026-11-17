package explorers

import (
	"fmt"
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

	t.Run("etherscan website and retired hosts use v2", func(t *testing.T) {
		cases := []struct {
			domain  string
			chainID uint64
		}{
			{"https://etherscan.io/", 1},
			{"https://api.bscscan.com/api", 56},
			{"https://berascan.com/", 80094},
			{"https://api.etherscan.io/v2/api?chainid=143", 143},
		}
		for _, tc := range cases {
			ee := New(KindEtherscan, tc.domain, "k", tc.chainID)
			urls := ee.contractInfoURLs(addr)
			if len(urls) != 1 {
				t.Fatalf("%s URLs: %v", tc.domain, urls)
			}
			want := fmt.Sprintf("https://api.etherscan.io/v2/api?chainid=%d&module=contract&action=getsourcecode", tc.chainID)
			if !strings.HasPrefix(urls[0], want) {
				t.Fatalf("%s URL: %s", tc.domain, urls[0])
			}
			if strings.Contains(urls[0], "etherscan.io/api") || strings.Contains(urls[0], "bscscan.com") || strings.Contains(urls[0], "/api/api") {
				t.Fatalf("%s still targets a retired host: %s", tc.domain, urls[0])
			}
			assertNoOtherFamilyPaths(t, urls)
		}

		ee := New(KindEtherscan, "https://scan.example/api", "k", 56)
		urls := ee.contractInfoURLs(addr)
		if len(urls) != 2 || strings.Contains(urls[0], "chainid=") || !strings.Contains(urls[1], "chainid=56") {
			t.Fatalf("non-etherscan clone should keep its own host: %v", urls)
		}
		if strings.Contains(urls[0], "api.etherscan.io") {
			t.Fatalf("non-etherscan clone was rewritten: %v", urls)
		}
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
