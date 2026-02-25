package txanalyzer

import (
	"strings"
	"sync"

	jarviscommon "github.com/tranvictor/jarvis/common"
	. "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/reader"
	"github.com/tranvictor/jarvis/util/addrbook"
)

// ERC20Info holds the token metadata discovered for a contract address.
type ERC20Info struct {
	Decimal uint64
	Symbol  string
}

// cachedERC20 wraps an ERC20Info so that a nil info (not an ERC20) can be
// distinguished from an unchecked address (absent from the map).
type cachedERC20 struct {
	info *ERC20Info // nil means "checked and not ERC20"
}

// AnalysisContext is the per-session knowledge base for a single jarvis run.
//
// It accumulates facts discovered during analysis (e.g. which contracts are
// ERC20 tokens, their decimals and symbols) and caches them in memory so that
// the same network lookup is never repeated within a single session.
//
// The underlying util/cache package already persists lookups to
// ~/.jarvis/cache.json between runs, so AnalysisContext adds only the
// fast in-memory layer on top.
type AnalysisContext struct {
	Network  Network
	Resolver addrbook.AddressResolver

	// reader is stored for future enrichment queries that need direct RPC
	// access (e.g. slot reads, multicall batching).
	reader reader.Reader

	mu    sync.RWMutex
	erc20 map[string]cachedERC20 // keyed by lower-case address
}

// NewAnalysisContext creates a fresh AnalysisContext using the default
// (production) address resolver backed by the local address databases.
func NewAnalysisContext(r reader.Reader, network Network) *AnalysisContext {
	return NewAnalysisContextWithResolver(r, network, addrbook.NewDefault(network))
}

// NewAnalysisContextWithResolver creates a fresh AnalysisContext with a
// custom AddressResolver. Use this in tests to inject an addrbook.Map
// (or any other deterministic implementation) instead of the local databases.
func NewAnalysisContextWithResolver(r reader.Reader, network Network, res addrbook.AddressResolver) *AnalysisContext {
	return &AnalysisContext{
		Network:  network,
		Resolver: res,
		reader:   r,
		erc20:    make(map[string]cachedERC20),
	}
}

// GetJarvisAddress resolves addr using the context's AddressResolver. All
// analyzer methods should call this instead of util.GetJarvisAddress directly
// so that the resolver can be swapped out in tests.
func (ctx *AnalysisContext) GetJarvisAddress(addr string) jarviscommon.Address {
	return ctx.Resolver.Resolve(addr)
}

// ERC20InfoFor returns token metadata for addr if the address is a known ERC20
// token, or nil otherwise. Results are cached in memory for the session
// lifetime; the underlying util functions also persist to disk across runs.
func (ctx *AnalysisContext) ERC20InfoFor(addr string) *ERC20Info {
	key := strings.ToLower(addr)

	ctx.mu.RLock()
	entry, found := ctx.erc20[key]
	ctx.mu.RUnlock()
	if found {
		return entry.info
	}

	// Not yet checked — fetch decimal and symbol in parallel.
	// util/cache provides disk persistence so subsequent calls are instant.
	var (
		decimal    uint64
		symbol     string
		decimalErr error
	)
	jarviscommon.RunParallel(
		func() error {
			decimal, decimalErr = util.GetERC20Decimal(addr, ctx.Network)
			return decimalErr
		},
		func() error {
			symbol, _ = util.GetERC20Symbol(addr, ctx.Network)
			return nil
		},
	)
	if decimalErr != nil {
		ctx.mu.Lock()
		ctx.erc20[key] = cachedERC20{info: nil}
		ctx.mu.Unlock()
		return nil
	}

	info := &ERC20Info{Decimal: decimal, Symbol: symbol}
	ctx.mu.Lock()
	ctx.erc20[key] = cachedERC20{info: info}
	ctx.mu.Unlock()
	return info
}
