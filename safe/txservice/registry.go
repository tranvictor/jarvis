package txservice

import (
	"fmt"
	"os"
	"strings"
)

// defaultURLs maps Safe-supported chain IDs to the canonical Safe Transaction
// Service base URL hosted at safe.global. Keep this list in sync with
// https://docs.safe.global/core-api/transaction-service-supported-networks.
//
// For chains not listed here, set the SAFE_TX_SERVICE_URL environment variable
// (applies to all chains) or SAFE_TX_SERVICE_URL_<chainId> (overrides for a
// single chain) to point at a self-hosted or private deployment.
var defaultURLs = map[uint64]string{
	1:        "https://safe-transaction-mainnet.safe.global",
	10:       "https://safe-transaction-optimism.safe.global",
	56:       "https://safe-transaction-bsc.safe.global",
	100:      "https://safe-transaction-gnosis-chain.safe.global",
	137:      "https://safe-transaction-polygon.safe.global",
	250:      "https://safe-transaction-fantom.safe.global",
	8453:     "https://safe-transaction-base.safe.global",
	42161:    "https://safe-transaction-arbitrum.safe.global",
	43114:    "https://safe-transaction-avalanche.safe.global",
	59144:    "https://safe-transaction-linea.safe.global",
	534352:   "https://safe-transaction-scroll.safe.global",
	1101:     "https://safe-transaction-zkevm.safe.global",
	11155111: "https://safe-transaction-sepolia.safe.global",
	84532:    "https://safe-transaction-base-sepolia.safe.global",
}

// URLForChain returns the Safe Transaction Service base URL for chainID,
// honoring environment overrides:
//
//   - SAFE_TX_SERVICE_URL_<chainID> wins over everything
//   - SAFE_TX_SERVICE_URL is used as a global override / unknown-chain fallback
//   - otherwise the bundled default for the chain is returned
//
// The returned URL has no trailing slash.
func URLForChain(chainID uint64) (string, error) {
	if v := strings.TrimSpace(os.Getenv(fmt.Sprintf("SAFE_TX_SERVICE_URL_%d", chainID))); v != "" {
		return strings.TrimRight(v, "/"), nil
	}
	if v := strings.TrimSpace(os.Getenv("SAFE_TX_SERVICE_URL")); v != "" {
		return strings.TrimRight(v, "/"), nil
	}
	if v, ok := defaultURLs[chainID]; ok {
		return v, nil
	}
	return "", fmt.Errorf(
		"no Safe Transaction Service URL is configured for chain %d. "+
			"Set SAFE_TX_SERVICE_URL_%d or SAFE_TX_SERVICE_URL to point at a deployment",
		chainID, chainID,
	)
}
