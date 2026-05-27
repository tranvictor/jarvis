package erc7730

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// DescriptorSource is the lookup surface match.go uses to walk
// candidate descriptors. The real implementation lives in registry.go;
// tests can pass a static slice through a simple in-memory source.
type DescriptorSource interface {
	// FindByContract returns descriptors whose context.contract
	// includes a deployment matching (chainID, address). The address
	// is compared case-insensitively. May return zero or many
	// descriptors; selector matching narrows further.
	FindByContract(chainID uint64, address string) []*Descriptor
	// FindByEIP712 returns descriptors whose context.eip712 might
	// apply to td. The matcher does the final exact-comparison.
	FindByEIP712(td *apitypes.TypedData) []*Descriptor
}

// domainChainID returns the typed-data domain's chainId as a uint64,
// or 0 if unset. apitypes.TypedDataDomain.ChainId is a
// *math.HexOrDecimal256 which is a typed *big.Int; we cast to access
// Uint64.
func domainChainID(td *apitypes.TypedData) uint64 {
	if td.Domain.ChainId == nil {
		return 0
	}
	return (*big.Int)(td.Domain.ChainId).Uint64()
}

// ContractMatchInput is everything required to pick a descriptor for
// an EVM transaction-style signing request.
type ContractMatchInput struct {
	ChainID uint64
	// To is the destination contract (or proxy) address — lower-case hex.
	To string
	// Calldata is the full transaction data (including selector). May
	// be empty for pure-value transfers (clear signing only ever
	// matches when calldata starts with a known selector).
	Calldata []byte
	// ImplFor optionally resolves a proxy address to the implementation
	// it points to. When non-nil and the registry has a descriptor
	// for the resolved implementation (but not the proxy itself), the
	// implementation descriptor is used. Errors and unsupported proxy
	// shapes are silently ignored.
	ImplFor func(addr string) (string, bool)
}

// MatchedFormat is the outcome of picking a single (descriptor,
// format) pair plus the resolved selector key — used for downstream
// rendering.
type MatchedFormat struct {
	Descriptor *Descriptor
	// FormatKey is the descriptor's `display.formats` key that
	// matched (the human-readable ABI signature).
	FormatKey string
	// Format is the rendering specification.
	Format *Format
	// ParamNames are the parameter names extracted from FormatKey,
	// in declaration order. Used as the path-resolver's primary key
	// when the underlying ABI has anonymous params.
	ParamNames []string
	// ParamTypes are the canonical Solidity type names (with names
	// dropped), in declaration order. Used for re-decoding via
	// go-ethereum's abi package when needed.
	ParamTypes []string
}

// FindContractMatch walks src for a descriptor that binds to in.To on
// in.ChainID and whose `display.formats` selector matches the first 4
// bytes of in.Calldata. Returns nil when no descriptor applies — that
// is the normal "unknown contract" case, not an error.
func FindContractMatch(src DescriptorSource, in ContractMatchInput) *MatchedFormat {
	if len(in.Calldata) < 4 {
		return nil
	}
	selector := in.Calldata[:4]

	candidates := src.FindByContract(in.ChainID, in.To)

	// Optional proxy resolution: if the direct lookup misses but the
	// caller can resolve the proxy to an implementation we know
	// about, retry with that implementation. We keep the original
	// (proxy) chainId so the user-facing context stays accurate.
	if len(candidates) == 0 && in.ImplFor != nil {
		if impl, ok := in.ImplFor(in.To); ok && !strings.EqualFold(impl, in.To) {
			candidates = src.FindByContract(in.ChainID, impl)
		}
	}

	for _, d := range candidates {
		for key, f := range d.Display.Formats {
			parsed, err := parseFormatKey(key)
			if err != nil {
				continue
			}
			if !bytesEqual(parsed.Selector, selector) {
				continue
			}
			return &MatchedFormat{
				Descriptor: d,
				FormatKey:  key,
				Format:     f,
				ParamNames: parsed.ParamNames,
				ParamTypes: parsed.ParamTypes,
			}
		}
	}
	return nil
}

// EIP712MatchInput is the dual of ContractMatchInput for typed-data.
type EIP712MatchInput struct {
	TypedData *apitypes.TypedData
}

// FindEIP712Match walks src for a descriptor whose eip712 context
// matches td's domain (by exact field match and/or domainSeparator),
// then picks the display.formats entry whose typeHash equals the
// message's primary type hash.
func FindEIP712Match(src DescriptorSource, in EIP712MatchInput) *MatchedFormat {
	if in.TypedData == nil {
		return nil
	}
	td := in.TypedData

	// Compute the domain separator once; descriptors that bind by
	// domainSeparator just compare against this hash.
	domainHash, err := td.HashStruct("EIP712Domain", td.Domain.Map())
	if err != nil {
		return nil
	}
	gotSep := common.BytesToHash(domainHash).Hex()

	candidates := src.FindByEIP712(td)

	for _, d := range candidates {
		if d.Context.EIP712 == nil {
			continue
		}
		if !eip712Matches(d.Context.EIP712, td, gotSep) {
			continue
		}
		// Now find the format entry whose encodeType matches the
		// message's primary type. encodeType for primaryType T is
		// the canonical signature string T plus all referenced
		// structs sorted alphabetically — exactly what go-ethereum
		// builds in apitypes.TypedData.EncodeType.
		want, err := encodeType(td)
		if err != nil {
			continue
		}
		wantHash := crypto.Keccak256(want)

		for key, f := range d.Display.Formats {
			if crypto.Keccak256Hash([]byte(key)) == common.BytesToHash(wantHash) {
				parsed, _ := parseEIP712FormatKey(key)
				return &MatchedFormat{
					Descriptor: d,
					FormatKey:  key,
					Format:     f,
					ParamNames: parsed.ParamNames,
					ParamTypes: parsed.ParamTypes,
				}
			}
			// Lenient match: strip optional whitespace so the
			// descriptor author's typing style doesn't bite us.
			if normalizeType([]byte(key)) == string(want) {
				parsed, _ := parseEIP712FormatKey(key)
				return &MatchedFormat{
					Descriptor: d,
					FormatKey:  key,
					Format:     f,
					ParamNames: parsed.ParamNames,
					ParamTypes: parsed.ParamTypes,
				}
			}
		}
	}
	return nil
}

func encodeType(td *apitypes.TypedData) ([]byte, error) {
	bs := td.EncodeType(td.PrimaryType)
	return []byte(bs), nil
}

// eip712Matches returns true when the descriptor's eip712 context
// permits binding to this typed data. The spec requires us to
// verify every key the descriptor lists; missing keys are unconstrained.
func eip712Matches(ctx *EIP712Ctx, td *apitypes.TypedData, gotDomainSep string) bool {
	if ctx.DomainSeparator != "" {
		if !equalHex(ctx.DomainSeparator, gotDomainSep) {
			return false
		}
	}
	if ctx.Domain != nil {
		if ctx.Domain.Name != "" && ctx.Domain.Name != td.Domain.Name {
			return false
		}
		if ctx.Domain.Version != "" && ctx.Domain.Version != td.Domain.Version {
			return false
		}
		if ctx.Domain.ChainID != 0 && ctx.Domain.ChainID != domainChainID(td) {
			return false
		}
		if ctx.Domain.VerifyingContract != "" {
			if !strings.EqualFold(ctx.Domain.VerifyingContract, td.Domain.VerifyingContract) {
				return false
			}
		}
	}
	if len(ctx.Deployments) > 0 {
		ok := false
		for _, dep := range ctx.Deployments {
			if dep.ChainID == domainChainID(td) &&
				strings.EqualFold(dep.Address, td.Domain.VerifyingContract) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// ── format-key parsing ──────────────────────────────────────────────────────

type parsedFormatKey struct {
	Name       string
	Selector   []byte
	ParamNames []string
	ParamTypes []string
}

// parseFormatKey parses a `display.formats` key for a contract
// descriptor into its name + 4-byte selector + (names, types).
//
// Accepted shapes (per the spec):
//
//	transfer(address to,uint256 value)
//	submitOrder((address token,uint256 amount,uint256 price) order,bytes32 salt)
//	airdrop(address[] recipients,uint256[3] values)
func parseFormatKey(s string) (parsedFormatKey, error) {
	name, params, err := splitNameAndParams(s)
	if err != nil {
		return parsedFormatKey{}, err
	}
	names, types, err := splitTopLevelParams(params)
	if err != nil {
		return parsedFormatKey{}, err
	}
	canonical := make([]string, len(types))
	for i, t := range types {
		canonical[i] = canonicalizeABIType(t)
	}
	typeOnly := name + "(" + strings.Join(canonical, ",") + ")"
	sel := crypto.Keccak256([]byte(typeOnly))[:4]
	return parsedFormatKey{Name: name, Selector: sel, ParamNames: names, ParamTypes: canonical}, nil
}

// canonicalizeABIType returns the canonical Solidity signature form of
// a single parameter type — names removed from any nested tuples,
// whitespace stripped, array suffix preserved. Tuples can nest
// arbitrarily ((a,(b,c[]))[]), so this recurses.
//
// Examples:
//
//	"address"                                      -> "address"
//	"address[]"                                    -> "address[]"
//	"(address token,uint256 amount)"               -> "(address,uint256)"
//	"(address token,uint256 amount) order"         -> "(address,uint256)"
//	"((address t,uint256 a) inner,bytes32 salt)"   -> "((address,uint256),bytes32)"
func canonicalizeABIType(t string) string {
	t = strings.TrimSpace(t)
	// Strip trailing identifier ("(...) name" or "uint256 amount").
	if name, typ, err := splitTypeAndName(t); err == nil && name != "" {
		t = strings.TrimSpace(typ)
	}
	// Detect array suffix like "[]" or "[N]" attached to a tuple.
	if !strings.HasPrefix(t, "(") {
		return strings.ReplaceAll(t, " ", "")
	}
	// Find the matching closing paren for the leading (.
	depth := 0
	closeIdx := -1
	for i, c := range t {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				closeIdx = i
			}
		}
		if closeIdx >= 0 {
			break
		}
	}
	if closeIdx < 0 {
		return t
	}
	inner := t[1:closeIdx]
	suffix := strings.ReplaceAll(t[closeIdx+1:], " ", "")
	parts, _ := splitTopLevelTypes(inner)
	for i, p := range parts {
		parts[i] = canonicalizeABIType(p)
	}
	return "(" + strings.Join(parts, ",") + ")" + suffix
}

// splitTopLevelTypes is splitTopLevelParams without the name parsing —
// used by canonicalizeABIType where each segment is already a bare
// (sometimes named) type.
func splitTopLevelTypes(s string) ([]string, error) {
	depthRound, depthSquare := 0, 0
	last := 0
	out := []string{}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depthRound++
		case ')':
			depthRound--
		case '[':
			depthSquare++
		case ']':
			depthSquare--
		case ',':
			if depthRound == 0 && depthSquare == 0 {
				out = append(out, s[last:i])
				last = i + 1
			}
		}
	}
	if last < len(s) {
		out = append(out, s[last:])
	}
	return out, nil
}

// parseEIP712FormatKey handles primary-type encodings. The shape is
// `T(field1 type1,field2 type2,…)Other(...)Foo(...)`. We only need
// the primary type's field list for path-resolution purposes; the
// trailing referenced types are looked up via the typedData.Types map
// at render time.
func parseEIP712FormatKey(s string) (parsedFormatKey, error) {
	// Take only up to the first ')' — that's the primary type.
	end := strings.IndexByte(s, ')')
	if end < 0 {
		return parsedFormatKey{}, fmt.Errorf("erc7730: malformed eip712 key %q", s)
	}
	name, params, err := splitNameAndParams(s[:end+1])
	if err != nil {
		return parsedFormatKey{}, err
	}
	// EIP-712 params: order matters; names and types are
	// space-separated. Tuples can't nest at the field level (they
	// reference other types by name) so the parser is simpler.
	parts := splitTopLevelEIP712(params)
	names := make([]string, 0, len(parts))
	types := make([]string, 0, len(parts))
	for _, p := range parts {
		// Note: EIP-712 uses `<type> <name>` (TYPE first), not the
		// Solidity `<type> <name>` convention. Same syntax though.
		sp := strings.LastIndexByte(strings.TrimSpace(p), ' ')
		if sp < 0 {
			return parsedFormatKey{}, fmt.Errorf("erc7730: bad eip712 param %q", p)
		}
		types = append(types, strings.TrimSpace(p[:sp]))
		names = append(names, strings.TrimSpace(p[sp+1:]))
	}
	return parsedFormatKey{Name: name, ParamNames: names, ParamTypes: types}, nil
}

// splitNameAndParams takes "funcName(<...>)" and returns
// ("funcName", "<...>").
func splitNameAndParams(s string) (string, string, error) {
	lp := strings.IndexByte(s, '(')
	if lp < 0 || !strings.HasSuffix(s, ")") {
		return "", "", fmt.Errorf("erc7730: malformed signature %q", s)
	}
	return s[:lp], s[lp+1 : len(s)-1], nil
}

// splitTopLevelParams splits a flat parameter list at top-level
// commas (those not nested inside parentheses or brackets) and pulls
// out the trailing identifier as the parameter name.
func splitTopLevelParams(s string) (names []string, types []string, err error) {
	depthRound, depthSquare := 0, 0
	last := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depthRound++
		case ')':
			depthRound--
		case '[':
			depthSquare++
		case ']':
			depthSquare--
		case ',':
			if depthRound == 0 && depthSquare == 0 {
				n, t, err := splitTypeAndName(s[last:i])
				if err != nil {
					return nil, nil, err
				}
				names = append(names, n)
				types = append(types, t)
				last = i + 1
			}
		}
	}
	if last < len(s) {
		n, t, err := splitTypeAndName(s[last:])
		if err != nil {
			return nil, nil, err
		}
		names = append(names, n)
		types = append(types, t)
	}
	return names, types, nil
}

// splitTypeAndName takes one param like " address to " or
// "(address token,uint256 amount) order" and returns (name, type).
// The type retains its original nesting; only the trailing identifier
// after the last space (outside any brackets) is treated as the name.
func splitTypeAndName(s string) (name string, typ string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", fmt.Errorf("erc7730: empty param")
	}
	// Find the last top-level space.
	depthRound, depthSquare := 0, 0
	last := -1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depthRound++
		case ')':
			depthRound--
		case '[':
			depthSquare++
		case ']':
			depthSquare--
		case ' ', '\t':
			if depthRound == 0 && depthSquare == 0 {
				last = i
			}
		}
	}
	if last < 0 {
		// Anonymous param (e.g. bare types from a typedef-style
		// descriptor). Permitted by the spec for cases where the
		// caller doesn't care about the name.
		return "", s, nil
	}
	return strings.TrimSpace(s[last+1:]), strings.TrimSpace(s[:last]), nil
}

// splitTopLevelEIP712 splits an EIP-712 primary-type body at top-level
// commas. Simpler than Solidity tuples because EIP-712 references
// other types by name only.
func splitTopLevelEIP712(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// normalizeType strips redundant whitespace so encodeType comparison
// becomes whitespace-tolerant.
func normalizeType(b []byte) string {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c == ' ' || c == '\t' || c == '\n' {
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalHex(a, b string) bool {
	return strings.EqualFold(stripHex(a), stripHex(b))
}

func stripHex(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	return s
}

// hexDecode decodes a 0x-prefixed (or bare) hex string, lower-cased
// length-tolerant, padded on the left to even length. Used by the
// resolver and the registry index when normalising domain separators.
func hexDecode(s string) ([]byte, error) {
	s = stripHex(s)
	if len(s)%2 == 1 {
		s = "0" + s
	}
	return hex.DecodeString(s)
}
