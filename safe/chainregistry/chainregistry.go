// Package chainregistry maintains jarvis's knowledge of which EVM chains
// Safe supports and where their Transaction Service lives, without
// hand-maintaining a hardcoded list.
//
// The registry layers three sources in priority order:
//
//  1. Live snapshot cached on disk under safe:chains:v1. Populated by
//     `jarvis msig chains refresh` (or a future auto-refresh on stale
//     cache). This is the authoritative source for users who have
//     refreshed at least once.
//  2. Built-in baseline: a frozen snapshot of the Safe-supported chains
//     known at jarvis build time. Guarantees offline users (airgapped,
//     no mainnet connectivity, Safe Config Service unreachable, etc.)
//     still get correct URLs for the most common chains.
//  3. Live fetch on explicit Refresh() from the Safe Config Service
//     (https://safe-client.safe.global/v1/chains). The merged result is
//     written back to the disk cache.
//
// All public lookups (ByChainID, ByShortName, All) merge the cached
// snapshot on top of the baseline — this way, adding a new chain via
// refresh overrides anything baked into the binary.
package chainregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tranvictor/jarvis/util/cache"
)

// DefaultEndpoint is the Safe Config Service endpoint that returns the
// paginated list of Safe-supported chains. It is overridden only in
// tests via SetEndpoint.
const DefaultEndpoint = "https://safe-client.safe.global/v1/chains"

const (
	cacheKey     = "safe:chains:v1"
	cacheTTL     = 7 * 24 * time.Hour
	fetchTimeout = 10 * time.Second
	pageLimit    = 200 // safe upper bound; the API returned <100 chains as of 2026-04.
)

// ChainInfo is the minimal subset jarvis uses. The upstream payload has
// many more fields (RPC URIs, gas settings, block explorer templates,
// etc.) but we deliberately keep this record small — anything bigger
// would force us to re-snapshot on every schema change.
type ChainInfo struct {
	ChainID            uint64 `json:"chain_id"`
	ShortName          string `json:"short_name"`
	TransactionService string `json:"transaction_service,omitempty"`
}

// snapshot is the on-disk representation we store in util/cache.
type snapshot struct {
	FetchedAt time.Time            `json:"fetched_at"`
	Chains    map[uint64]ChainInfo `json:"chains"`
}

// builtIn is the frozen baseline shipped with the binary. It should be
// a conservative subset — only chains where the URL is stable enough
// that it's reasonable to hardcode. Use `jarvis msig chains refresh`
// to pick up anything newer.
//
// Source: https://docs.safe.global/core-api/transaction-service-supported-networks
// (as of 2026-04). Keep in rough sync but don't sweat staleness: the
// live refresh is the intended way to stay current.
var builtIn = map[uint64]ChainInfo{
	1:          {ChainID: 1, ShortName: "eth", TransactionService: "https://safe-transaction-mainnet.safe.global"},
	10:         {ChainID: 10, ShortName: "oeth", TransactionService: "https://safe-transaction-optimism.safe.global"},
	56:         {ChainID: 56, ShortName: "bnb", TransactionService: "https://safe-transaction-bsc.safe.global"},
	100:        {ChainID: 100, ShortName: "gno", TransactionService: "https://safe-transaction-gnosis-chain.safe.global"},
	137:        {ChainID: 137, ShortName: "matic", TransactionService: "https://safe-transaction-polygon.safe.global"},
	146:        {ChainID: 146, ShortName: "sonic", TransactionService: ""},
	250:        {ChainID: 250, ShortName: "ftm", TransactionService: "https://safe-transaction-fantom.safe.global"},
	1101:       {ChainID: 1101, ShortName: "zkevm", TransactionService: "https://safe-transaction-zkevm.safe.global"},
	5000:       {ChainID: 5000, ShortName: "mantle", TransactionService: ""},
	8453:       {ChainID: 8453, ShortName: "base", TransactionService: "https://safe-transaction-base.safe.global"},
	42161:      {ChainID: 42161, ShortName: "arb1", TransactionService: "https://safe-transaction-arbitrum.safe.global"},
	42220:      {ChainID: 42220, ShortName: "celo", TransactionService: ""},
	43114:      {ChainID: 43114, ShortName: "avax", TransactionService: "https://safe-transaction-avalanche.safe.global"},
	59144:      {ChainID: 59144, ShortName: "linea", TransactionService: "https://safe-transaction-linea.safe.global"},
	81457:      {ChainID: 81457, ShortName: "blast", TransactionService: ""},
	534352:     {ChainID: 534352, ShortName: "scr", TransactionService: "https://safe-transaction-scroll.safe.global"},
	11155111:   {ChainID: 11155111, ShortName: "sep", TransactionService: "https://safe-transaction-sepolia.safe.global"},
	84532:      {ChainID: 84532, ShortName: "basesep", TransactionService: "https://safe-transaction-base-sepolia.safe.global"},
	1313161554: {ChainID: 1313161554, ShortName: "aurora", TransactionService: ""},
}

var (
	mu       sync.RWMutex
	loaded   bool
	snapData snapshot
	endpoint = DefaultEndpoint
)

// load merges the built-in baseline with any cached snapshot on first
// access. Safe to call multiple times — subsequent calls are a no-op.
func load() {
	mu.Lock()
	defer mu.Unlock()
	if loaded {
		return
	}
	loaded = true

	snapData = snapshot{Chains: map[uint64]ChainInfo{}}
	for k, v := range builtIn {
		snapData.Chains[k] = v
	}
	if raw, ok := cache.GetCache(cacheKey); ok {
		var s snapshot
		if err := json.Unmarshal([]byte(raw), &s); err == nil && len(s.Chains) > 0 {
			for k, v := range s.Chains {
				snapData.Chains[k] = v
			}
			snapData.FetchedAt = s.FetchedAt
		}
	}
}

// ByChainID returns the ChainInfo for chainID (if the registry knows
// it). The second return is false when the chain is unknown.
func ByChainID(chainID uint64) (ChainInfo, bool) {
	load()
	mu.RLock()
	defer mu.RUnlock()
	ci, ok := snapData.Chains[chainID]
	return ci, ok
}

// ByShortName is the inverse lookup: given an EIP-3770 short name
// (e.g. "eth", "arb1", "matic"), return the ChainInfo. Match is
// case-insensitive.
func ByShortName(shortName string) (ChainInfo, bool) {
	load()
	mu.RLock()
	defer mu.RUnlock()
	needle := strings.ToLower(strings.TrimSpace(shortName))
	for _, ci := range snapData.Chains {
		if strings.ToLower(ci.ShortName) == needle {
			return ci, true
		}
	}
	return ChainInfo{}, false
}

// All returns a slice snapshot of every chain currently known to the
// registry (built-in + cached merged). The slice is freshly allocated
// so callers may sort or mutate it freely.
func All() []ChainInfo {
	load()
	mu.RLock()
	defer mu.RUnlock()
	out := make([]ChainInfo, 0, len(snapData.Chains))
	for _, ci := range snapData.Chains {
		out = append(out, ci)
	}
	return out
}

// FetchedAt reports when the cached snapshot was last successfully
// refreshed from the Safe Config Service. Returns the zero time when
// the registry only has the built-in baseline (i.e. Refresh has never
// been run successfully).
func FetchedAt() time.Time {
	load()
	mu.RLock()
	defer mu.RUnlock()
	return snapData.FetchedAt
}

// CacheExpired reports whether our cached snapshot is older than the
// TTL (or missing entirely). Intended for callers that want to nudge
// the user with a "consider running `jarvis msig chains refresh`" hint
// when lookups are likely stale.
func CacheExpired() bool {
	t := FetchedAt()
	return t.IsZero() || time.Since(t) > cacheTTL
}

// SetEndpoint overrides the Safe Config Service endpoint used by
// Refresh. Intended for tests.
func SetEndpoint(u string) { endpoint = u }

// Refresh fetches the latest chain list from Safe's Config Service,
// follows pagination to completion, and persists the merged snapshot
// in the on-disk cache. It returns the number of chains loaded on
// success; any error leaves the cache untouched.
//
// The in-memory snapshot is updated on success so subsequent lookups
// in the same process see the fresh data without re-reading the cache.
func Refresh(ctx context.Context) (int, error) {
	load()

	client := &http.Client{Timeout: fetchTimeout}
	url := fmt.Sprintf("%s?limit=%d", endpoint, pageLimit)
	all := map[uint64]ChainInfo{}

	for url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return 0, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("User-Agent", "jarvis-chainregistry/1.0")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return 0, fmt.Errorf("fetch %s: %w", url, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return 0, fmt.Errorf("read %s: %w", url, readErr)
		}
		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("safe config service %s returned %d: %s",
				url, resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var page struct {
			Next    string `json:"next"`
			Results []struct {
				ChainID            string `json:"chainId"`
				ShortName          string `json:"shortName"`
				TransactionService string `json:"transactionService"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return 0, fmt.Errorf("decode safe config service response: %w", err)
		}
		for _, r := range page.Results {
			id, err := strconv.ParseUint(r.ChainID, 10, 64)
			if err != nil {
				continue // skip malformed ids rather than fail the whole refresh
			}
			all[id] = ChainInfo{
				ChainID:            id,
				ShortName:          strings.ToLower(strings.TrimSpace(r.ShortName)),
				TransactionService: strings.TrimRight(strings.TrimSpace(r.TransactionService), "/"),
			}
		}
		// The upstream API occasionally returns absolute URLs for `next`
		// that drop the scheme back to http:// (CloudFront / proxy
		// quirks). Upgrade only when the caller-configured endpoint is
		// https — that way local test servers using http://127.0.0.1
		// continue to work.
		next := strings.TrimSpace(page.Next)
		if next != "" && strings.HasPrefix(next, "http://") && strings.HasPrefix(endpoint, "https://") {
			next = "https://" + strings.TrimPrefix(next, "http://")
		}
		url = next
	}

	if len(all) == 0 {
		return 0, fmt.Errorf("safe config service returned no chains")
	}

	s := snapshot{FetchedAt: time.Now().UTC(), Chains: all}
	raw, err := json.Marshal(s)
	if err != nil {
		return 0, fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := cache.SetCache(cacheKey, string(raw)); err != nil {
		return 0, fmt.Errorf("write cache: %w", err)
	}

	mu.Lock()
	for k, v := range all {
		snapData.Chains[k] = v
	}
	snapData.FetchedAt = s.FetchedAt
	mu.Unlock()

	return len(all), nil
}
