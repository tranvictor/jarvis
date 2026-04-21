package txservice

import (
	"fmt"
	"os"
	"strings"

	"github.com/tranvictor/jarvis/safe/chainregistry"
)

// URLForChain returns the Safe Transaction Service base URL for chainID,
// honoring environment overrides in this priority order:
//
//  1. SAFE_TX_SERVICE_URL_<chainID> — per-chain override, wins over everything.
//  2. SAFE_TX_SERVICE_URL — global override / unknown-chain fallback.
//  3. The chain registry (built-in baseline merged with any cached
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

	if ci, ok := chainregistry.ByChainID(chainID); ok && ci.TransactionService != "" {
		return ci.TransactionService, nil
	}

	hint := ""
	if chainregistry.CacheExpired() {
		hint = " Run `jarvis msig chains refresh` to pull the latest list from Safe."
	}
	return "", fmt.Errorf(
		"no Safe Transaction Service URL is configured for chain %d.%s "+
			"Alternatively, set SAFE_TX_SERVICE_URL_%d or SAFE_TX_SERVICE_URL to point at a deployment",
		chainID, hint, chainID,
	)
}
