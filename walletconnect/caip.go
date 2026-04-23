package walletconnect

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// CAIP-2 / CAIP-10 wrappers scoped to the "eip155" namespace. The WC v2
// protocol negotiates chains and accounts using these opaque strings,
// so all gateway-visible chain identifiers flow through these helpers
// rather than raw uint64 chain IDs.
//
//   chain:   "eip155:<chainID>"
//   account: "eip155:<chainID>:<0xaddr>"
//
// We never produce or consume non-eip155 namespaces; WC's broader CAIP
// support (cosmos, solana, ...) is out of scope for jarvis.

const caipEIP155Namespace = "eip155"

// ChainString returns the CAIP-2 form of an EIP-155 chain ID.
func ChainString(chainID uint64) string {
	return fmt.Sprintf("%s:%d", caipEIP155Namespace, chainID)
}

// AccountString returns the CAIP-10 form for (chainID, address). The
// address is lower-cased because WC peers compare these strings
// byte-for-byte and mixed-case addresses break that expectation.
func AccountString(chainID uint64, address string) string {
	return fmt.Sprintf(
		"%s:%d:%s",
		caipEIP155Namespace, chainID, strings.ToLower(address),
	)
}

// ParseChain extracts the EIP-155 chain ID from a CAIP-2 string. It
// returns an error for any non-eip155 namespace, which the caller
// should translate into a user-facing "unsupported chain" message.
func ParseChain(s string) (uint64, error) {
	ns, body, ok := splitCAIP(s, 1)
	if !ok {
		return 0, fmt.Errorf("CAIP-2 chain must be '<ns>:<ref>', got %q", s)
	}
	if ns != caipEIP155Namespace {
		return 0, fmt.Errorf(
			"unsupported CAIP-2 namespace %q (jarvis only handles eip155)", ns)
	}
	id, err := strconv.ParseUint(body, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("CAIP-2 chain reference %q is not a uint: %w", body, err)
	}
	return id, nil
}

// ParseAccount extracts (chainID, address) from a CAIP-10 account
// string. The address is not validated for checksum — lowercased input
// is accepted as-is.
func ParseAccount(s string) (chainID uint64, address common.Address, err error) {
	ns, body, ok := splitCAIP(s, 2)
	if !ok {
		return 0, common.Address{}, fmt.Errorf(
			"CAIP-10 account must be '<ns>:<ref>:<addr>', got %q", s)
	}
	if ns != caipEIP155Namespace {
		return 0, common.Address{}, fmt.Errorf(
			"unsupported CAIP-10 namespace %q", ns)
	}
	parts := strings.SplitN(body, ":", 2)
	if len(parts) != 2 {
		return 0, common.Address{}, fmt.Errorf(
			"CAIP-10 account is missing address component: %q", s)
	}
	id, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, common.Address{}, fmt.Errorf(
			"CAIP-10 chain reference %q is not a uint: %w", parts[0], err)
	}
	if !common.IsHexAddress(parts[1]) {
		return 0, common.Address{}, fmt.Errorf(
			"CAIP-10 address %q is not a valid 0x-hex address", parts[1])
	}
	return id, common.HexToAddress(parts[1]), nil
}

// splitCAIP splits s into (namespace, remainder) on the first ':' and
// confirms the remainder contains at least `minColons` additional ':'
// separators. Returns ok=false for malformed input.
func splitCAIP(s string, minColons int) (ns, rest string, ok bool) {
	i := strings.Index(s, ":")
	if i <= 0 || i == len(s)-1 {
		return "", "", false
	}
	ns = s[:i]
	rest = s[i+1:]
	if strings.Count(s, ":") < minColons {
		return "", "", false
	}
	return ns, rest, true
}
