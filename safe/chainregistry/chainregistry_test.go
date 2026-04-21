package chainregistry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/util/cache"
)

func TestBuiltInLookups(t *testing.T) {
	ci, ok := ByChainID(1)
	if !ok {
		t.Fatal("expected mainnet in built-in registry")
	}
	if ci.ShortName != "eth" {
		t.Errorf("mainnet short name = %q, want %q", ci.ShortName, "eth")
	}
	if !strings.Contains(ci.TransactionService, "safe.global") {
		t.Errorf("mainnet tx service = %q, expected a safe.global URL", ci.TransactionService)
	}

	ci, ok = ByShortName("arb1")
	if !ok || ci.ChainID != 42161 {
		t.Errorf("arb1 -> %+v, want chain id 42161", ci)
	}

	// ByShortName is case-insensitive.
	ci, ok = ByShortName("ETH")
	if !ok || ci.ChainID != 1 {
		t.Errorf("uppercase ETH -> %+v, want chain id 1", ci)
	}

	if _, ok := ByChainID(99999999); ok {
		t.Error("unknown chain id should return ok=false")
	}
	if _, ok := ByShortName("not-a-real-chain"); ok {
		t.Error("unknown short name should return ok=false")
	}
}

// TestRefreshWithFakeEndpoint exercises the pagination loop and cache
// behaviour using a local httptest server. It asserts the registry
// picks up freshly-fetched chains AND preserves the built-in baseline
// for chains the fake server didn't return.
func TestRefreshWithFakeEndpoint(t *testing.T) {
	// Point the on-disk cache at a per-test temp file so we don't
	// require write access to the real ~/.jarvis/cache.json (and so
	// parallel test runs don't stomp on each other).
	origCachePath := cache.CACHE_PATH
	cache.CACHE_PATH = filepath.Join(t.TempDir(), "cache.json")
	defer func() { cache.CACHE_PATH = origCachePath }()

	var serverBase string
	page := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case 0:
			fmt.Fprintf(w, `{
				"count": 2,
				"next": "%s?page=2",
				"results": [
					{"chainId": "7777", "shortName": "testa", "transactionService": "https://sts-a.example/"}
				]
			}`, serverBase)
		case 1:
			fmt.Fprintln(w, `{
				"count": 2,
				"next": null,
				"results": [
					{"chainId": "8888", "shortName": "testb", "transactionService": "https://sts-b.example"}
				]
			}`)
		default:
			t.Fatalf("unexpected extra call at page %d", page)
		}
		page++
	}))
	defer srv.Close()
	serverBase = srv.URL

	SetEndpoint(srv.URL)
	defer SetEndpoint(DefaultEndpoint)

	n, err := Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if n != 2 {
		t.Errorf("got %d chains from refresh, want 2", n)
	}

	ci, ok := ByChainID(7777)
	if !ok {
		t.Fatal("expected chain 7777 after refresh")
	}
	if ci.TransactionService != "https://sts-a.example" {
		t.Errorf("trailing slash not stripped: got %q", ci.TransactionService)
	}
	if ci2, ok := ByShortName("testb"); !ok || ci2.ChainID != 8888 {
		t.Errorf("short name lookup after refresh: %+v (ok=%v)", ci2, ok)
	}

	// The built-in baseline must still win for chains the fake server
	// didn't return (refresh merges, it doesn't replace wholesale).
	if _, ok := ByChainID(1); !ok {
		t.Error("built-in mainnet entry disappeared after refresh")
	}

	if FetchedAt().IsZero() {
		t.Error("FetchedAt should be non-zero after a successful refresh")
	}
	if CacheExpired() {
		t.Error("CacheExpired should be false immediately after refresh")
	}
}
