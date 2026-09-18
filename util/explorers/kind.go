package explorers

import (
	"fmt"
	"net/url"
	"strings"
)

// Kind is an explorer family. Contract ABI/source lookup is per-family:
// each class has its own URL shape, and we never append another family's
// paths onto a host (Robinscan is Robinhood-only; Blockscout REST is not
// an Etherscan fallback).
//
// Sourcify is not a Kind. It is a chain-agnostic exact-match overlay tried
// after the family's own URLs, before SimilarMatch / verified-twin follow.
type Kind string

const (
	// KindEtherscan is Etherscan and Etherscan-clone module=contract APIs.
	// v2 unified (api.etherscan.io/v2) requires chainid; v1 per-host APIs
	// (api.bscscan.com, …) do not.
	KindEtherscan Kind = "etherscan"
	// KindBlockscout is Blockscout REST (/api/v2/smart-contracts/:addr,
	// and /api/v2/smart-contract/:addr when the configured domain already
	// ends in /api/v2) plus the etherscan-compat module=contract endpoint.
	KindBlockscout Kind = "blockscout"
	// KindRoutescan is Routescan's etherscan-compat gateway, where the
	// chain is already in the path
	// (api.routescan.io/v2/network/{mainnet|testnet}/evm/{chainId}/etherscan/).
	KindRoutescan Kind = "routescan"
	// KindRobinscan is Robinscan's REST GET /api/contracts/:addr
	// (isVerified + sourceFiles). It is Robinhood-specific, not a generic
	// fallback for every chain. robin.etherscan.io is Etherscan v2 with
	// chainid=4663, not Robinscan.
	KindRobinscan Kind = "robinscan"
)

// ParseKind accepts a config/CLI explorer_kind value. Empty is valid and
// means "infer from the API URL".
func ParseKind(s string) (Kind, error) {
	k := Kind(strings.ToLower(strings.TrimSpace(s)))
	switch k {
	case "", KindEtherscan, KindBlockscout, KindRoutescan, KindRobinscan:
		return k, nil
	default:
		return "", fmt.Errorf("unknown explorer kind %q (want etherscan, blockscout, routescan, or robinscan)", s)
	}
}

// InferKind guesses the explorer family from a block-explorer API URL.
// Explicit explorer_kind on a network config always wins.
func InferKind(apiURL string) Kind {
	raw := strings.TrimSpace(apiURL)
	if raw == "" {
		return KindEtherscan
	}
	lower := strings.ToLower(raw)
	host := hostOf(raw)

	if strings.Contains(host, "robinscan") || strings.Contains(lower, "robinscan") {
		return KindRobinscan
	}
	if strings.Contains(host, "routescan") || strings.Contains(host, "snowtrace") ||
		strings.Contains(lower, "routescan.io") || strings.Contains(lower, "snowtrace.io") {
		return KindRoutescan
	}
	if strings.Contains(host, "blockscout") || strings.Contains(lower, "blockscout") {
		return KindBlockscout
	}
	// Blockscout / OP-stack explorers (Bitfi, self-hosted) expose REST at
	// …/api/v2. Etherscan's unified API is https://api.etherscan.io/v2 —
	// path "/v2", not "/api/v2" — so this does not steal Etherscan v2.
	if strings.Contains(lower, "/api/v2") && !etherscanHost(host, lower) {
		return KindBlockscout
	}
	return KindEtherscan
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return strings.ToLower(raw)
	}
	if u.Host != "" {
		return strings.ToLower(u.Host)
	}
	return strings.ToLower(raw)
}

func etherscanHost(host, lowerURL string) bool {
	return strings.Contains(host, "etherscan.io") ||
		strings.Contains(host, "etherscan.com") ||
		strings.Contains(lowerURL, "etherscan.io") ||
		strings.Contains(lowerURL, "etherscan.com")
}

// New builds an explorer of the given family. An empty kind is inferred
// from domain.
func New(kind Kind, domain, apiKey string, chainID uint64) *EtherscanLikeExplorer {
	if kind == "" {
		kind = InferKind(domain)
	}
	return &EtherscanLikeExplorer{
		Kind:    kind,
		Domain:  domain,
		APIKey:  apiKey,
		ChainID: chainID,
	}
}
