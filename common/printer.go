package common

import (
	"fmt"
	"strings"

	"github.com/tranvictor/jarvis/config"
)

// uintMaxSentinels maps decimal representations of common uintN.max values
// to their canonical "infinity" label. Smart contracts widely use these as
// "do the maximum / unlimited / withdraw all" sentinels (Aave withdraw, ERC20
// approve, Uniswap deadline, etc.) and rendering them as the literal 78-digit
// number actively obscures intent. Exact-match only — we never want to round
// values that just happen to be close to the max.
var uintMaxSentinels = map[string]string{
	// 2**256 - 1
	"115792089237316195423570985008687907853269984665640564039457584007913129639935": "uint256.max (∞)",
	// 2**128 - 1
	"340282366920938463463374607431768211455": "uint128.max",
	// 2**64 - 1
	"18446744073709551615": "uint64.max",
	// 2**32 - 1
	"4294967295": "uint32.max",
}

// MaxUintLabel returns the canonical "uintN.max" label and true when value
// matches one of the well-known max-uint sentinels. Empty string and false
// otherwise. Cheap (map lookup) and safe to call on every integer render.
func MaxUintLabel(value string) (string, bool) {
	label, ok := uintMaxSentinels[value]
	return label, ok
}

// ReadableNumber renders a big integer as "raw (grouped)" where grouped has
// thousands separators: "1000000000 (1,000,000,000)". Well-known max-uint
// sentinels render as their label. Short numbers are returned as is.
func ReadableNumber(value string) string {
	if label, ok := MaxUintLabel(value); ok {
		return label
	}
	if len(strings.TrimPrefix(value, "-")) <= 4 {
		return value
	}
	return fmt.Sprintf("%s (%s)", value, GroupDigits(value))
}

// GroupDigits inserts thousands separators into the integer part of a
// decimal number string: "1234567.891" → "1,234,567.891". Non-numeric input
// is returned unchanged.
func GroupDigits(value string) string {
	sign := ""
	if strings.HasPrefix(value, "-") {
		sign, value = "-", value[1:]
	}
	intPart, frac := value, ""
	if i := strings.IndexByte(value, '.'); i >= 0 {
		intPart, frac = value[:i], value[i:]
	}
	for _, c := range intPart {
		if c < '0' || c > '9' {
			return sign + intPart + frac
		}
	}
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return sign + b.String() + frac
}

// CompactAmount shortens a human token amount for dense read-only views:
// thousands separators on the integer part and the fraction truncated to four
// decimals (or four significant digits when the integer part is zero).
// Precision is for the signing card and JSON; this is for scanning.
func CompactAmount(human string) string {
	intPart, frac := human, ""
	if i := strings.IndexByte(human, '.'); i >= 0 {
		intPart, frac = human[:i], human[i+1:]
	}
	if frac == "" {
		return GroupDigits(intPart)
	}
	keep := 4
	if strings.Trim(intPart, "-0") == "" {
		keep = len(frac) - len(strings.TrimLeft(frac, "0")) + 4
	}
	if len(frac) > keep {
		frac = frac[:keep]
	}
	frac = strings.TrimRight(frac, "0")
	if frac == "" {
		return GroupDigits(intPart)
	}
	return GroupDigits(intPart) + "." + frac
}

// PlainAddress formats an Address as a plain string with no ANSI color codes.
// Use this when the result will be stored in a data structure or serialized to
// JSON so that consumers don't receive terminal markup.
func PlainAddress(addr Address) string {
	if addr.Address == "" {
		return ""
	}
	if addr.Decimal != 0 {
		return fmt.Sprintf("%s (%s - %d)", addr.Address, addr.Desc, addr.Decimal)
	}
	if addr.Desc != "" {
		return fmt.Sprintf("%s (%s)", addr.Address, addr.Desc)
	}
	return addr.Address
}

// IsKnownAddress reports whether addr carries a usable description from the
// address book / token list. "unknown" is what the analyzer fills in when no
// entry matched, so it counts as not known.
func IsKnownAddress(addr Address) bool {
	return addr.Desc != "" && addr.Desc != "unknown"
}

// ShortAddress abbreviates a hex address to its first four and last four hex
// digits ("0x9642…5D4E"). Anything too short to abbreviate is returned as is.
// Never use this on a signing screen: lookalike-address attacks rely on the
// middle of the address being invisible.
func ShortAddress(hex string) string {
	if len(hex) <= 13 {
		return hex
	}
	return hex[:6] + "…" + hex[len(hex)-4:]
}

// NameFirst formats an address for dense, read-only views. Known addresses
// lead with their name and put the hex in parentheses; unknown ones lead with
// the hex followed by "(unknown)". full selects the complete hex over the
// ShortAddress form.
func NameFirst(addr Address, full bool) string {
	if addr.Address == "" {
		return ""
	}
	hex := addr.Address
	if !full {
		hex = ShortAddress(hex)
	}
	if IsZeroAddress(addr.Address) {
		return hex + " (zero address)"
	}
	if !IsKnownAddress(addr) {
		return hex
	}
	return fmt.Sprintf("%s (%s)", addr.Desc, hex)
}

// IsZeroAddress reports whether hex is 0x0000…0000, the conventional mint /
// burn counterparty and "no address" sentinel.
func IsZeroAddress(hex string) bool {
	if !strings.HasPrefix(hex, "0x") || len(hex) != 42 {
		return false
	}
	return strings.Trim(hex[2:], "0") == ""
}

// VerboseAddress formats an Address for terminal display. The description is
// wrapped in ANSI color via NameWithColor. Do NOT use the output as data
// (e.g. JSON) — use PlainAddress for that.
func VerboseAddress(addr Address) string {
	if addr.Address == "" {
		return ""
	}
	if addr.Decimal != 0 {
		return fmt.Sprintf(
			"%s (%s)",
			addr.Address,
			NameWithColor(fmt.Sprintf("%s - %d", addr.Desc, addr.Decimal)),
		)
	}
	return fmt.Sprintf("%s (%s)", addr.Address, NameWithColor(addr.Desc))
}

// PlainValue returns a human-readable string for a single decoded ABI value
// with no ANSI color codes. Use in build/data phases.
func PlainValue(value Value) string {
	switch value.Kind {
	case DisplayAddress:
		return PlainAddress(*value.Address)
	case DisplayToken:
		if label, ok := MaxUintLabel(value.Raw); ok {
			if value.Token.Symbol != "" {
				return fmt.Sprintf("%s (%s, all %s)", value.Raw, label, value.Token.Symbol)
			}
			return fmt.Sprintf("%s (%s)", value.Raw, label)
		}
		human := BigToFloatString(StringToBig(value.Raw), value.Token.Decimal)
		if value.Token.Symbol != "" {
			return fmt.Sprintf("%s (%s %s)", value.Raw, human, value.Token.Symbol)
		}
		return fmt.Sprintf("%s (%s)", value.Raw, human)
	case DisplayInteger:
		return ReadableNumber(value.Raw)
	default:
		return value.Raw
	}
}

func DebugPrintf(format string, a ...any) (n int, err error) {
	if config.Debug {
		return fmt.Printf(format, a...)
	}

	return 0, nil
}
