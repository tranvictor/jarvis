// Package ens provides minimal, isolated Ethereum Name Service forward
// resolution (name -> address). It is deliberately scoped to plain .eth
// names resolved against the canonical ENS registry on Ethereum mainnet:
//
//   - No CCIP-Read / ENSIP-10 gateway support (so alt-TLDs like
//     user.base.eth / foo.linea.eth won't resolve here — that needs an
//     HTTP gateway path we don't ship yet).
//   - No reverse resolution (addr -> name).
//   - No multicoin (ENSIP-9/11) — the "default" ETH address is returned.
//
// The package is self-contained: it depends only on go-ethereum, jarvis's
// reader, and jarvis's on-disk cache. It does NOT import jarvis/util, so
// higher-level integration code in util can call into it without
// creating an import cycle.
package ens

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/tranvictor/jarvis/util/cache"
	"github.com/tranvictor/jarvis/util/reader"
)

// MainnetRegistryAddress is the ENS registry deployed on Ethereum
// mainnet. All .eth resolutions start here. (Previous registry
// deployments exist at 0x314159... but the 2020 "registry with
// fallback" at this address is the canonical one for new resolutions.)
const MainnetRegistryAddress = "0x00000000000C2E074eC69A0dFb2997BA6C7d2e1e"

// Error sentinels. Wrap, don't compare on message strings.
var (
	// ErrNotAnENSName means the input didn't look like a .eth domain;
	// the caller should fall through to its usual address-book / hex
	// handling without printing a user-facing warning.
	ErrNotAnENSName = fmt.Errorf("not an ENS name")

	// ErrNoResolver means the registry has no resolver configured for
	// this name (i.e. the name is not registered or its owner never
	// set a resolver). Distinct from ErrNoAddress because the UX for
	// "unregistered" vs "registered but unconfigured" differs.
	ErrNoResolver = fmt.Errorf("ens: no resolver configured for name")

	// ErrNoAddress means the resolver returned 0x0 for addr(node). The
	// name exists but hasn't been pointed at an ETH address yet.
	ErrNoAddress = fmt.Errorf("ens: resolver returned zero address")
)

// ensLabelRe restricts labels to ASCII letter-digit-hyphen-underscore.
// Full UTS-46 / ENSIP-15 normalisation is out of scope for this minimal
// resolver; in practice the vast majority of .eth names are ASCII and
// rejecting odd inputs is safer than misinterpreting them.
var ensLabelRe = regexp.MustCompile(`^[a-z0-9_-]+$`)

// IsLikelyENSName reports whether s looks like a .eth domain jarvis
// should attempt to resolve via ENS. The check is intentionally
// conservative:
//
//   - must end with ".eth" (case-insensitive)
//   - must have at least one label before .eth
//   - each label must be [a-z0-9_-]+ after lowercasing
//
// This lets obvious inputs like "alice.eth" and "foo.bar.eth" through
// while rejecting hex addresses, address-book labels that happen to
// contain dots, and fuzzy search strings.
func IsLikelyENSName(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || !strings.HasSuffix(s, ".eth") {
		return false
	}
	labels := strings.Split(s, ".")
	// "eth" alone (len 1) isn't a resolvable name; need at least "x.eth"
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if !ensLabelRe.MatchString(l) {
			return false
		}
	}
	return true
}

// Namehash computes the EIP-137 namehash of a DNS-style name.
//
//	namehash("")        = 0x0000...0000
//	namehash("eth")     = keccak256(0x00...00 || keccak256("eth"))
//	namehash("foo.eth") = keccak256(namehash("eth") || keccak256("foo"))
//
// The input is lowercased before hashing. Full UTS-46 normalisation is
// NOT applied; callers should already have validated the input via
// IsLikelyENSName, which only permits ASCII labels.
func Namehash(name string) [32]byte {
	var node [32]byte
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return node
	}
	labels := strings.Split(name, ".")
	// Walk right-to-left: TLD first, then progressively more specific.
	for i := len(labels) - 1; i >= 0; i-- {
		labelHash := crypto.Keccak256([]byte(labels[i]))
		combined := append([]byte{}, node[:]...)
		combined = append(combined, labelHash...)
		copy(node[:], crypto.Keccak256(combined))
	}
	return node
}

// Resolver performs forward ENS resolution (name -> address).
type Resolver interface {
	Resolve(name string) (common.Address, error)
}

// NewMainnetResolver returns a Resolver that calls out to the ENS
// registry on Ethereum mainnet via r. Callers are expected to pass a
// reader backed by mainnet nodes; resolving via a non-mainnet reader
// will silently fail because the registry contract does not exist at
// the same address on other chains.
func NewMainnetResolver(r *reader.EthReader) Resolver {
	return &ethReaderResolver{
		r:         r,
		registry:  common.HexToAddress(MainnetRegistryAddress),
		regABI:    registryABI,
		resABI:    resolverABI,
	}
}

type ethReaderResolver struct {
	r        *reader.EthReader
	registry common.Address
	regABI   *abi.ABI
	resABI   *abi.ABI
}

// Resolve implements name -> address. Results are cached on disk under
// "ens:v1:<name>" so subsequent jarvis invocations don't repeat the
// registry + resolver calls. Nothing is cached on failure: failures are
// usually transient (node hiccup, offline) and the user will retry.
func (e *ethReaderResolver) Resolve(name string) (common.Address, error) {
	if !IsLikelyENSName(name) {
		return common.Address{}, ErrNotAnENSName
	}
	name = strings.ToLower(strings.TrimSpace(name))

	cacheKey := "ens:v1:" + name
	if cached, ok := cache.GetCache(cacheKey); ok && cached != "" {
		return common.HexToAddress(cached), nil
	}

	node := Namehash(name)

	// Registry: resolver(bytes32 node) -> address
	rawResolver, err := e.r.ReadContractToBytes(
		-1, "0x0000000000000000000000000000000000000000",
		e.registry.Hex(), e.regABI, "resolver", node,
	)
	if err != nil {
		return common.Address{}, fmt.Errorf("ens registry.resolver: %w", err)
	}
	var resolver common.Address
	if err := e.regABI.UnpackIntoInterface(&resolver, "resolver", rawResolver); err != nil {
		return common.Address{}, fmt.Errorf("ens registry.resolver unpack: %w", err)
	}
	if resolver == (common.Address{}) {
		return common.Address{}, ErrNoResolver
	}

	// Resolver: addr(bytes32 node) -> address
	rawAddr, err := e.r.ReadContractToBytes(
		-1, "0x0000000000000000000000000000000000000000",
		resolver.Hex(), e.resABI, "addr", node,
	)
	if err != nil {
		return common.Address{}, fmt.Errorf("ens resolver.addr: %w", err)
	}
	var out common.Address
	if err := e.resABI.UnpackIntoInterface(&out, "addr", rawAddr); err != nil {
		return common.Address{}, fmt.Errorf("ens resolver.addr unpack: %w", err)
	}
	if out == (common.Address{}) {
		return common.Address{}, ErrNoAddress
	}

	_ = cache.SetCache(cacheKey, out.Hex())
	return out, nil
}

// Minimal ABI fragments — only the two methods we actually call.
// Parsing them once at package init avoids paying the cost on every
// resolution.
var (
	registryABI = mustParseABI(`[{
		"name": "resolver",
		"type": "function",
		"stateMutability": "view",
		"inputs": [{"name":"node","type":"bytes32"}],
		"outputs":[{"name":"","type":"address"}]
	}]`)
	resolverABI = mustParseABI(`[{
		"name": "addr",
		"type": "function",
		"stateMutability": "view",
		"inputs": [{"name":"node","type":"bytes32"}],
		"outputs":[{"name":"","type":"address"}]
	}]`)
)

func mustParseABI(s string) *abi.ABI {
	a, err := abi.JSON(strings.NewReader(s))
	if err != nil {
		panic(fmt.Sprintf("ens: bad ABI fragment: %s", err))
	}
	return &a
}

// CachedLookup returns a previously-resolved address for name, if any.
// Useful when a caller wants to display provenance ("ens: alice.eth ->
// 0xABC") without triggering a network call or a fallback warning.
func CachedLookup(name string) (common.Address, bool) {
	if !IsLikelyENSName(name) {
		return common.Address{}, false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if v, ok := cache.GetCache("ens:v1:" + name); ok && v != "" {
		return common.HexToAddress(v), true
	}
	return common.Address{}, false
}

// Process-wide memoised resolver. The wiring in util/util.go constructs
// one via NewMainnetResolver on first use and shares it across calls.
// Lives here so tests can substitute a stub resolver without touching
// util's internal state.
var (
	defaultMu  sync.RWMutex
	defaultRes Resolver
)

// SetDefault installs the resolver subsequent Default() calls will
// return. Intended for integration points that own the mainnet reader
// (util.go's lazy init) and for tests that want to inject a fake.
// Re-calling replaces the previous resolver; tests that swap and
// restore should capture the previous value via Default() first.
func SetDefault(r Resolver) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultRes = r
}

// Default returns the globally-installed resolver, or nil if none has
// been installed yet. Call sites that can't live without ENS should
// handle nil gracefully by treating it as "resolution unavailable,
// fall through to address book".
func Default() Resolver {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultRes
}
