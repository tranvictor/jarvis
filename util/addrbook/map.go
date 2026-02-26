package addrbook

import (
	"strings"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

// Map is a lightweight AddressResolver for tests. It maps lower-cased
// Ethereum addresses to human-readable names; anything not in the map
// resolves to "unknown" without any network or database calls.
//
// Example:
//
//	r := resolver.Map{
//	    "0xd8da6bf26964af9d7eed9e03e53415d37aa96045": "Vitalik Buterin",
//	    "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48": "USDC",
//	}
type Map map[string]string

func (m Map) Resolve(addr string) jarviscommon.Address {
	if desc, ok := m[strings.ToLower(addr)]; ok {
		return jarviscommon.Address{Address: addr, Desc: desc}
	}
	return jarviscommon.Address{Address: addr, Desc: "unknown"}
}
