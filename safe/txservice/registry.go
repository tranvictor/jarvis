package txservice

import (
	"fmt"
	"os"
	"strings"

	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/safe/chainregistry"
)

// networkSafeTxServiceURL resolves the `safe_tx_service_url` a network
// config carries, or "" when jarvis doesn't know the chain or the field
// is unset. It is a variable so tests can stub the network layer instead
// of writing into the user's real ~/.jarvis/networks (mirrors
// chainregistry.SetEndpoint).
var networkSafeTxServiceURL = func(chainID uint64) string {
	// An unknown chain id is not an error here — it just means the network
	// isn't one jarvis knows about, so it can't contribute a URL and we
	// fall through to the registry.
	n, err := networks.GetNetworkByID(chainID)
	if err != nil {
		return ""
	}
	return n.GetSafeTxServiceURL()
}

// URLForChain returns the Safe Transaction Service base URL for chainID,
// honoring environment overrides in this priority order:
//
//  1. SAFE_TX_SERVICE_URL_<chainID> — per-chain override, wins over everything.
//  2. SAFE_TX_SERVICE_URL — global override / unknown-chain fallback.
//  3. The network's own `safe_tx_service_url` config field. This is how a
//     chain Safe doesn't list — a custom chain from ~/.jarvis/networks/ —
//     carries its (usually self-hosted) service persistently, rather than
//     depending on an env var being exported in every shell. It sits above
//     the registry so it can also correct a stale bundled URL.
//  4. The chain registry (built-in baseline merged with any cached
//     snapshot from the Safe Config Service — see safe/chainregistry).
//
// The returned URL has no trailing slash. When the chain is unknown
// AND no env override is set, the error message directs the user
// toward `jarvis msig chains refresh` so they can pick up chains added
// to Safe after this binary was built without needing a new release.
func URLForChain(chainID uint64) (string, error) {
	if v := strings.TrimSpace(os.Getenv(fmt.Sprintf("SAFE_TX_SERVICE_URL_%d", chainID))); v != "" {
		return strings.TrimRight(v, "/"), nil
	}
	if v := strings.TrimSpace(os.Getenv("SAFE_TX_SERVICE_URL")); v != "" {
		return strings.TrimRight(v, "/"), nil
	}

	if v := networkSafeTxServiceURL(chainID); v != "" {
		return v, nil
	}

	if ci, ok := chainregistry.ByChainID(chainID); ok && ci.TransactionService != "" {
		return ci.TransactionService, nil
	}

	hint := ""
	if chainregistry.CacheExpired() {
		hint = " Run `jarvis msig chains refresh` to pull the latest list from Safe."
	}
	return "", fmt.Errorf(
		"no Safe Transaction Service URL is configured for chain %d.%s "+
			"Alternatively, set SAFE_TX_SERVICE_URL_%d or SAFE_TX_SERVICE_URL to point at a deployment, "+
			`or add "safe_tx_service_url" to the network's config in ~/.jarvis/networks/`,
		chainID, hint, chainID,
	)
}
