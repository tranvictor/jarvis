package addrbook

import (
	"fmt"
	"strings"

	bleve "github.com/tranvictor/jarvis/bleve"
	jarviscommon "github.com/tranvictor/jarvis/common"
	db "github.com/tranvictor/jarvis/db"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util/cache"
)

// Default is the production AddressResolver. It owns the canonical logic for
// mapping a raw Ethereum address to a human-readable jarviscommon.Address:
//
//  1. Name lookup — queries the local bleve full-text index and the embedded
//     address database to find a human-readable description.
//  2. ERC20 decimal enrichment — reads the decimal value from the on-disk
//     cache (populated by earlier ERC20InfoFor / IsERC20 calls) so that the
//     display layer can render "USDC - 6" suffixes without a network round-trip.
//
// This type intentionally does NOT import the util package to avoid the
// util → util/addrbook → util import cycle. All dependencies are either
// lower-level packages (bleve, db, util/cache) or the stdlib.
type Default struct {
	network jarvisnetworks.Network
}

// NewDefault returns a Default resolver for the given network.
func NewDefault(network jarvisnetworks.Network) AddressResolver {
	return Default{network: network}
}

// Resolve looks up addr in the local address databases and enriches the result
// with ERC20 decimal metadata when available from the on-disk cache.
func (r Default) Resolve(addr string) jarviscommon.Address {
	// ERC20 enrichment — cache-only, no network call.
	// Caches are pre-populated by AnalysisContext.ERC20InfoFor during tx
	// analysis, by util.IsERC20, or by util.GetERC20Symbol/GetERC20Decimal.
	var decimal int64
	var symbol string
	var erc20Detected bool

	if s, found := cache.GetCache(fmt.Sprintf("%s_symbol", addr)); found && s != "" {
		symbol = s
		erc20Detected = true
		decimal, _ = cache.GetInt64Cache(fmt.Sprintf("%s_decimal", addr))
	} else if isERC20, found := cache.GetBoolCache(fmt.Sprintf("%s_isERC20", addr)); found && isERC20 {
		erc20Detected = true
		decimal, _ = cache.GetInt64Cache(fmt.Sprintf("%s_decimal", addr))
	}

	resolvedAddr, name, err := lookupName(addr)
	if err != nil {
		if erc20Detected && symbol != "" {
			return jarviscommon.Address{Address: addr, Desc: symbol + " token", Decimal: decimal}
		}
		return jarviscommon.Address{Address: addr, Desc: "unknown"}
	}

	if erc20Detected {
		return jarviscommon.Address{Address: resolvedAddr, Desc: name, Decimal: decimal}
	}
	return jarviscommon.Address{Address: resolvedAddr, Desc: name}
}

// lookupName searches the bleve full-text index and the embedded DB for addr,
// returning the first match. It mirrors the logic of
// util.getRelevantAddressesFromDatabases without creating an import cycle.
func lookupName(addr string) (resolvedAddr, name string, err error) {
	seen := map[string]bool{}
	var addrs []string
	var names []string

	for _, a := range mustBleveFirst(addr) {
		key := strings.ToLower(a.Address)
		if !seen[key] {
			seen[key] = true
			addrs = append(addrs, a.Address)
			names = append(names, a.Desc)
		}
	}
	for _, a := range mustDBFirst(addr) {
		key := strings.ToLower(a.Address)
		if !seen[key] {
			seen[key] = true
			addrs = append(addrs, a.Address)
			names = append(names, a.Desc)
		}
	}

	if len(addrs) == 0 {
		return "", "", fmt.Errorf("address not found for %q", addr)
	}
	return addrs[0], names[0], nil
}

func mustBleveFirst(addr string) []bleve.AddressDesc {
	results, _ := bleve.GetAddresses(addr)
	return results
}

func mustDBFirst(addr string) []db.AddressDesc {
	results, _ := db.GetAddresses(addr)
	return results
}
