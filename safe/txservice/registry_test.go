package txservice

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/util/cache"
)

// stubNetworkURL replaces the network-config lookup for the duration of a
// test so we never read (or need) the user's ~/.jarvis/networks.
func stubNetworkURL(t *testing.T, urls map[uint64]string) {
	t.Helper()
	prev := networkSafeTxServiceURL
	networkSafeTxServiceURL = func(chainID uint64) string { return urls[chainID] }
	t.Cleanup(func() { networkSafeTxServiceURL = prev })
}

// isolateCache points the on-disk cache at a temp file so chainregistry
// lookups can't pick up (or clobber) the real ~/.jarvis/cache.json.
func isolateCache(t *testing.T) {
	t.Helper()
	prev := cache.CACHE_PATH
	cache.CACHE_PATH = filepath.Join(t.TempDir(), "cache.json")
	t.Cleanup(func() { cache.CACHE_PATH = prev })
}

func TestURLForChainNetworkConfig(t *testing.T) {
	isolateCache(t)
	stubNetworkURL(t, map[uint64]string{4663: "https://safe-tx.example.com"})

	got, err := URLForChain(4663)
	if err != nil {
		t.Fatalf("URLForChain(4663) failed: %s", err)
	}
	if got != "https://safe-tx.example.com" {
		t.Errorf("URLForChain(4663) = %q, want the network's configured URL", got)
	}
}

func TestURLForChainNetworkConfigBeatsRegistry(t *testing.T) {
	isolateCache(t)
	// Chain 1 is in the built-in registry; an explicit network config
	// should still win so a stale bundled URL can be corrected.
	stubNetworkURL(t, map[uint64]string{1: "https://my-mainnet-service.example.com"})

	got, err := URLForChain(1)
	if err != nil {
		t.Fatalf("URLForChain(1) failed: %s", err)
	}
	if got != "https://my-mainnet-service.example.com" {
		t.Errorf("URLForChain(1) = %q, want the network config to override the registry", got)
	}
}

func TestURLForChainEnvBeatsNetworkConfig(t *testing.T) {
	isolateCache(t)
	stubNetworkURL(t, map[uint64]string{4663: "https://from-config.example.com"})

	t.Setenv("SAFE_TX_SERVICE_URL_4663", "https://from-env.example.com/")
	got, err := URLForChain(4663)
	if err != nil {
		t.Fatalf("URLForChain(4663) failed: %s", err)
	}
	if got != "https://from-env.example.com" {
		t.Errorf("URLForChain(4663) = %q, want the env override (trailing slash trimmed)", got)
	}
}

func TestURLForChainFallsBackToRegistry(t *testing.T) {
	isolateCache(t)
	// No network config for chain 1 — the built-in registry answers.
	stubNetworkURL(t, map[uint64]string{})

	got, err := URLForChain(1)
	if err != nil {
		t.Fatalf("URLForChain(1) failed: %s", err)
	}
	if !strings.Contains(got, "safe.global") {
		t.Errorf("URLForChain(1) = %q, want the built-in registry URL", got)
	}
}

func TestURLForChainUnknownMentionsConfigField(t *testing.T) {
	isolateCache(t)
	stubNetworkURL(t, map[uint64]string{})

	_, err := URLForChain(4663)
	if err == nil {
		t.Fatal("expected an error for a chain with no service anywhere")
	}
	if !strings.Contains(err.Error(), "safe_tx_service_url") {
		t.Errorf("error %q should point the user at the network config field", err)
	}
}
