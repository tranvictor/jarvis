package erc7730

import (
	"strings"
	"sync"

	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/reader"
)

// JarvisHelpers wires the formatters to jarvis's existing
// address-book, ENS, ERC-20 metadata, and network registry. One
// JarvisHelpers per process is enough — the underlying jarvis caches
// already memoise both in-process and on-disk.
//
// We need a per-chain Reader for proxy resolution; that one ARG is
// the only non-trivial bit. Production callers (cmd/util,
// walletconnect/gateways) supply it through the constructor.
type JarvisHelpers struct {
	// readerFor is the per-chain RPC reader factory. May be nil —
	// proxy resolution then silently fails.
	readerFor func(chainID uint64) (reader.Reader, networks.Network, bool)
}

// NewJarvisHelpers returns a Helpers implementation backed by jarvis
// utilities. readerFor maps a chainID to a Reader for that chain
// (used for proxy resolution and on-chain token metadata lookups);
// passing a nil readerFor disables those features without breaking
// the rest of the helpers.
func NewJarvisHelpers(readerFor func(chainID uint64) (reader.Reader, networks.Network, bool)) *JarvisHelpers {
	return &JarvisHelpers{readerFor: readerFor}
}

// ResolveAddress returns the same "0xabc… (Label)" string the rest of
// jarvis prints. The chain matters only because the EnrichedResolver
// uses it for the explorer-side contract-name fetch.
func (h *JarvisHelpers) ResolveAddress(addr string, chainID uint64) string {
	net, ok := h.networkFor(chainID)
	if !ok {
		return addr
	}
	resolved := util.GetJarvisAddress(addr, net)
	if resolved.Desc == "" || resolved.Desc == "unknown" {
		return resolved.Address
	}
	return shortAddr(resolved.Address) + " (" + resolved.Desc + ")"
}

// TokenMetadata calls into the existing token-metadata cache. We
// look up symbol and decimal in parallel — same pattern as
// txanalyzer.ERC20InfoFor.
func (h *JarvisHelpers) TokenMetadata(addr string, chainID uint64) (uint64, string, bool) {
	net, ok := h.networkFor(chainID)
	if !ok {
		return 0, "", false
	}
	dec, err := util.GetERC20Decimal(addr, net)
	if err != nil {
		return 0, "", false
	}
	sym, _ := util.GetERC20Symbol(addr, net)
	return dec, sym, true
}

// NetworkName returns the jarvis-internal network name (which serves
// as a reasonable "human readable" chain name in our CLI context).
func (h *JarvisHelpers) NetworkName(chainID uint64) (string, bool) {
	net, ok := h.networkFor(chainID)
	if !ok {
		return "", false
	}
	return net.GetName(), true
}

// NativeSymbol returns the chain's native currency symbol (ETH, BNB, …).
func (h *JarvisHelpers) NativeSymbol(chainID uint64) string {
	net, ok := h.networkFor(chainID)
	if !ok {
		return ""
	}
	return net.GetNativeTokenSymbol()
}

// ImplementationOf resolves a proxy to its implementation on the
// given chain only. Used by Engine.implForChain during clear-sign
// descriptor matching.
func (h *JarvisHelpers) ImplementationOf(addr string, chainID uint64) (string, bool) {
	if h.readerFor == nil {
		return "", false
	}
	rd, _, ok := h.readerFor(chainID)
	if !ok {
		return "", false
	}
	er, ok := rd.(*reader.EthReader)
	if !ok {
		return "", false
	}
	impl, err := er.ImplementationOf(-1, addr)
	if err != nil || impl.Big().Sign() == 0 {
		return "", false
	}
	return strings.ToLower(impl.Hex()), true
}

// networkFor caches network lookups so we don't repeatedly walk the
// supported-networks slice.
var (
	netCacheMu sync.Mutex
	netCache   = map[uint64]networks.Network{}
)

func (h *JarvisHelpers) networkFor(chainID uint64) (networks.Network, bool) {
	netCacheMu.Lock()
	if n, ok := netCache[chainID]; ok {
		netCacheMu.Unlock()
		return n, true
	}
	netCacheMu.Unlock()

	n, err := networks.GetNetworkByID(chainID)
	if err != nil {
		return nil, false
	}
	netCacheMu.Lock()
	netCache[chainID] = n
	netCacheMu.Unlock()
	return n, true
}

// DefaultEngine returns an Engine wired with the standard jarvis
// production dependencies (the on-disk registry under
// ~/.jarvis/erc7730, the live network list, the address-book /
// ERC-20 caches, and proxy resolution via EthReader).
//
// The Engine is safe to construct on every signing call; the
// LocalRegistry caches its in-memory indexes for the process
// lifetime and concurrent reads are guarded inside the registry.
func DefaultEngine() *Engine {
	helpers := NewJarvisHelpers(jarvisReaderFor)
	return &Engine{
		Source:        sharedLocalRegistry(),
		Helpers:       helpers,
		AutoSyncEvery: defaultAutoSync,
	}
}

// jarvisReaderFor maps a chainID to a Reader and Network using the
// regular EthReader pipeline. Returns ok=false when the chain isn't
// configured locally — proxy resolution then degrades silently.
func jarvisReaderFor(chainID uint64) (reader.Reader, networks.Network, bool) {
	n, err := networks.GetNetworkByID(chainID)
	if err != nil {
		return nil, nil, false
	}
	rd, err := util.EthReader(n)
	if err != nil {
		return nil, n, false
	}
	// *reader.EthReader implements reader.Reader; the explicit
	// return type widening keeps callers depending on the interface.
	return rd, n, true
}

var (
	sharedRegistryOnce sync.Once
	sharedRegistry     *LocalRegistry
)

const defaultAutoSync = 0 // disabled by default; CLI `clearsign update` is the explicit path

func sharedLocalRegistry() *LocalRegistry {
	sharedRegistryOnce.Do(func() {
		sharedRegistry = NewLocalRegistry("")
	})
	return sharedRegistry
}

// SharedRegistry exposes the process-wide registry instance to CLI
// code (`jarvis clearsign update / add / list / show`) so it shares
// state with the Engine used at sign time.
func SharedRegistry() *LocalRegistry { return sharedLocalRegistry() }

func shortAddr(s string) string {
	if len(s) < 12 {
		return s
	}
	return s[:6] + "…" + s[len(s)-4:]
}
