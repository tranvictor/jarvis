package erc7730

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

// Helpers is the optional set of out-of-package lookups the formatters
// can use to enrich output. All methods are best-effort — they MAY
// return their zero-value result and the formatter will fall back to
// a sensible raw representation.
//
// Production callers wire this to jarvis util helpers (GetJarvisAddress,
// GetERC20Symbol/Decimal, networks.GetNetworkByID). Tests can pass
// a stub or nil.
type Helpers interface {
	// ResolveAddress turns an address into a (short hex, label) pair.
	// label may be empty for unknown addresses.
	ResolveAddress(addr string, chainID uint64) (display string)
	// TokenMetadata fetches decimals + symbol for an ERC-20 contract.
	// ok=false means "unknown / not an ERC-20", and the formatter
	// will fall back to printing the raw integer with a question
	// mark for the symbol.
	TokenMetadata(addr string, chainID uint64) (decimals uint64, symbol string, ok bool)
	// NetworkName returns the human-readable name for a chain id
	// (e.g. "Ethereum Mainnet"). Used by the `chainId` format.
	NetworkName(chainID uint64) (string, bool)
	// NativeSymbol returns the native currency symbol for chainID
	// (e.g. "ETH", "BNB"). Used by the `amount` format.
	NativeSymbol(chainID uint64) string
}

// FormattedField is the rendered result of applying a Field
// specification to a ResolvedValue. The Render() entry point assembles
// a list of these and hands them to the ui layer.
type FormattedField struct {
	Label string
	Value string
	// Warn is set when the formatter couldn't resolve the value
	// completely (e.g. token symbol unknown). The ui layer renders
	// such rows in yellow.
	Warn bool
}

// Formatter applies one Field spec to the value at field.Path and
// returns one or more FormattedField rows (one per array element when
// the field iterates).
type Formatter struct {
	Resolver *Resolver
	Helpers  Helpers
}

// FormatField is the entry point.
func (f *Formatter) FormatField(field Field) ([]FormattedField, error) {
	// Group field with nested fields: recurse, but prefix paths.
	if len(field.Fields) > 0 {
		return f.formatGroup(field)
	}

	// $ref expansion — resolve the referenced definition and merge.
	if field.Ref != "" {
		merged, err := f.resolveRef(field)
		if err != nil {
			return nil, err
		}
		field = merged
	}

	path, val, err := f.resolveFieldPath(field)
	if err != nil {
		return nil, err
	}

	if !shouldShow(field.Visibility(), val) {
		return nil, nil // skipped per descriptor's visibility rule
	}

	// Array iteration: when the path points to an array and the
	// format is per-element (no group), render each element under
	// the same label suffixed with [i].
	if val.Kind == ResolvedArray && !path.IsLeafSlice() {
		out := []FormattedField{}
		for i, elem := range val.Array {
			rendered, err := f.renderOne(field, elem)
			if err != nil {
				return nil, err
			}
			label := field.Label
			if label == "" {
				label = pathLabel(path)
			}
			out = append(out, FormattedField{
				Label: fmt.Sprintf("%s [%d]", label, i),
				Value: rendered.Value,
				Warn:  rendered.Warn,
			})
		}
		return out, nil
	}

	rendered, err := f.renderOne(field, val)
	if err != nil {
		return nil, err
	}
	if rendered.Label == "" {
		if field.Label != "" {
			rendered.Label = field.Label
		} else {
			rendered.Label = pathLabel(path)
		}
	}
	return []FormattedField{rendered}, nil
}

func (f *Formatter) resolveRef(field Field) (Field, error) {
	// $ref is a $.display.definitions.<name> path.
	p, err := ParsePath(field.Ref)
	if err != nil {
		return field, err
	}
	if p.Root != RootDesc || len(p.Segments) < 3 ||
		p.Segments[0].Name != "display" || p.Segments[1].Name != "definitions" {
		return field, fmt.Errorf("erc7730: only $.display.definitions.<name> $refs supported")
	}
	def, ok := f.Resolver.Descriptor.Display.Definitions[p.Segments[2].Name]
	if !ok {
		return field, fmt.Errorf("erc7730: definition %q not found", p.Segments[2].Name)
	}
	// Merge: caller's params override the definition's.
	merged := def
	merged.Path = field.Path // caller decides what path to bind to
	if field.Label != "" {
		merged.Label = field.Label
	}
	if field.Format != "" {
		merged.Format = field.Format
	}
	merged.Params = map[string]any{}
	for k, v := range def.Params {
		merged.Params[k] = v
	}
	for k, v := range field.Params {
		merged.Params[k] = v
	}
	merged.Visible = field.Visible
	return merged, nil
}

func (f *Formatter) resolveFieldPath(field Field) (Path, ResolvedValue, error) {
	// `value` literal short-circuit.
	if field.Value != nil {
		return Path{}, wrapAny(field.Value), nil
	}
	if field.Path == "" {
		return Path{}, ResolvedValue{}, fmt.Errorf("erc7730: field has neither path nor value")
	}
	path, err := ParsePath(field.Path)
	if err != nil {
		return Path{}, ResolvedValue{}, err
	}
	val, err := f.Resolver.Resolve(path)
	if err != nil {
		return Path{}, ResolvedValue{}, err
	}
	return path, val, nil
}

func (f *Formatter) renderOne(field Field, val ResolvedValue) (FormattedField, error) {
	format := field.Format
	if format == "" {
		format = defaultFormatFor(val)
	}
	switch format {
	case "raw":
		return FormattedField{Value: f.formatRaw(val)}, nil
	case "amount":
		return f.formatAmount(val), nil
	case "tokenAmount":
		return f.formatTokenAmount(field, val), nil
	case "addressName":
		return f.formatAddressName(field, val), nil
	case "date":
		return f.formatDate(field, val), nil
	case "duration":
		return FormattedField{Value: formatDuration(val)}, nil
	case "unit":
		return f.formatUnit(field, val), nil
	case "enum":
		return f.formatEnum(field, val), nil
	case "chainId":
		return f.formatChainID(val), nil
	case "nftName":
		return f.formatNFTName(field, val), nil
	case "calldata":
		// Embedded calldata is rendered as a raw 0x… string here;
		// the higher-level render.go inspects the calleePath and
		// triggers a recursive ClearSigned lookup if the underlying
		// contract is known. Falling through to raw is the safe
		// fallback when the embedded call can't be decoded.
		return FormattedField{Value: f.formatRaw(val)}, nil
	}
	// Unknown format — fall back to raw.
	return FormattedField{Value: f.formatRaw(val), Warn: true}, nil
}

func defaultFormatFor(v ResolvedValue) string {
	switch v.Kind {
	case ResolvedAddress:
		return "addressName"
	case ResolvedInt:
		return "raw"
	case ResolvedBytes:
		return "raw"
	}
	return "raw"
}

// ── individual formatters ────────────────────────────────────────────────

func (f *Formatter) formatRaw(v ResolvedValue) string {
	switch v.Kind {
	case ResolvedInt:
		return formatBigInt(v.Int)
	case ResolvedBytes:
		return "0x" + hex.EncodeToString(v.Bytes)
	case ResolvedString:
		return v.Str
	case ResolvedBool:
		return fmt.Sprintf("%t", v.Bool)
	case ResolvedAddress:
		return v.Str
	case ResolvedArray:
		parts := make([]string, len(v.Array))
		for i, e := range v.Array {
			parts[i] = f.formatRaw(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return ""
}

func (f *Formatter) formatAmount(v ResolvedValue) FormattedField {
	if v.Kind != ResolvedInt {
		return FormattedField{Value: f.formatRaw(v), Warn: true}
	}
	chainID := f.Resolver.Container.ChainID
	var sym string
	if f.Helpers != nil {
		sym = f.Helpers.NativeSymbol(chainID)
	}
	if sym == "" {
		sym = "ETH"
	}
	human := jarviscommon.BigToFloatString(v.Int, 18)
	return FormattedField{Value: fmt.Sprintf("%s %s", human, sym)}
}

func (f *Formatter) formatTokenAmount(field Field, v ResolvedValue) FormattedField {
	if v.Kind != ResolvedInt {
		return FormattedField{Value: f.formatRaw(v), Warn: true}
	}
	tokenAddr, _ := f.resolveTokenParam(field, "tokenPath", "token")
	threshold, hasThreshold := f.thresholdParam(field)
	message := stringParam(field, "message", "Unlimited")
	nativeAddrs := f.nativeCurrencyAddresses(field)

	// Above-threshold display.
	if hasThreshold && v.Int.Cmp(threshold) >= 0 {
		if tokenAddr != "" {
			if dec, sym, ok := f.tokenInfo(tokenAddr); ok && sym != "" {
				_ = dec
				return FormattedField{Value: fmt.Sprintf("%s %s", message, sym)}
			}
		}
		return FormattedField{Value: message}
	}

	// Native currency short-circuit.
	if tokenAddr != "" && containsCI(nativeAddrs, tokenAddr) {
		sym := ""
		if f.Helpers != nil {
			sym = f.Helpers.NativeSymbol(f.Resolver.Container.ChainID)
		}
		if sym == "" {
			sym = "ETH"
		}
		return FormattedField{Value: fmt.Sprintf("%s %s", jarviscommon.BigToFloatString(v.Int, 18), sym)}
	}

	// Standard ERC-20 path.
	if tokenAddr != "" {
		dec, sym, ok := f.tokenInfo(tokenAddr)
		if ok && sym != "" {
			return FormattedField{Value: fmt.Sprintf("%s %s", jarviscommon.BigToFloatString(v.Int, dec), sym)}
		}
	}
	// Fall back to raw integer + "?" symbol so the user knows the
	// token couldn't be identified.
	return FormattedField{Value: fmt.Sprintf("%s (Unknown token)", formatBigInt(v.Int)), Warn: true}
}

func (f *Formatter) formatAddressName(field Field, v ResolvedValue) FormattedField {
	addr := ""
	switch v.Kind {
	case ResolvedAddress:
		addr = v.Addr
	case ResolvedBytes:
		// Slice-of-bytes that happens to be 20 bytes wide — treat
		// as address (Permit2's tokenPath = "path.[0:20]" pattern).
		if len(v.Bytes) == 20 {
			addr = "0x" + hex.EncodeToString(v.Bytes)
		}
	case ResolvedString:
		addr = v.Str
	}
	if addr == "" {
		return FormattedField{Value: f.formatRaw(v), Warn: true}
	}
	if f.Helpers == nil {
		return FormattedField{Value: addr}
	}
	return FormattedField{Value: f.Helpers.ResolveAddress(addr, f.Resolver.Container.ChainID)}
}

func (f *Formatter) formatDate(field Field, v ResolvedValue) FormattedField {
	if v.Kind != ResolvedInt {
		return FormattedField{Value: f.formatRaw(v), Warn: true}
	}
	enc := stringParam(field, "encoding", "timestamp")
	n := v.Int.Int64()
	switch enc {
	case "timestamp":
		t := time.Unix(n, 0).UTC()
		return FormattedField{Value: t.Format("2006-01-02T15:04:05Z")}
	case "blockheight":
		// We can't convert blockheight to a real timestamp without
		// a node round-trip; print the height and tag it so the
		// user knows it's not a wall-clock value.
		return FormattedField{Value: fmt.Sprintf("block #%d", n), Warn: true}
	}
	return FormattedField{Value: f.formatRaw(v), Warn: true}
}

func formatDuration(v ResolvedValue) string {
	if v.Kind != ResolvedInt {
		return ""
	}
	secs := v.Int.Int64()
	if secs < 0 {
		secs = 0
	}
	h, m, s := secs/3600, (secs%3600)/60, secs%60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func (f *Formatter) formatUnit(field Field, v ResolvedValue) FormattedField {
	if v.Kind != ResolvedInt {
		return FormattedField{Value: f.formatRaw(v), Warn: true}
	}
	base := stringParam(field, "base", "")
	decimals := intParam(field, "decimals", 0)
	prefix, _ := field.Params["prefix"].(bool)
	val := jarviscommon.BigToFloatString(v.Int, uint64(decimals))
	if prefix {
		// SI prefix conversion is left as a future enhancement —
		// we render the raw scaled value with the base unit.
	}
	return FormattedField{Value: strings.TrimSpace(val + " " + base)}
}

func (f *Formatter) formatEnum(field Field, v ResolvedValue) FormattedField {
	key := ""
	switch v.Kind {
	case ResolvedInt:
		key = v.Int.String()
	case ResolvedString:
		key = v.Str
	default:
		return FormattedField{Value: f.formatRaw(v), Warn: true}
	}
	refStr, _ := field.Params["$ref"].(string)
	if refStr == "" {
		return FormattedField{Value: key, Warn: true}
	}
	p, err := ParsePath(refStr)
	if err != nil {
		return FormattedField{Value: key, Warn: true}
	}
	if p.Root != RootDesc || len(p.Segments) < 2 ||
		p.Segments[0].Name != "metadata" || p.Segments[1].Name != "enums" {
		return FormattedField{Value: key, Warn: true}
	}
	if len(p.Segments) < 3 {
		return FormattedField{Value: key, Warn: true}
	}
	enum, ok := f.Resolver.Descriptor.Metadata.Enums[p.Segments[2].Name]
	if !ok {
		return FormattedField{Value: key, Warn: true}
	}
	if disp, ok := enum[key]; ok {
		return FormattedField{Value: disp}
	}
	return FormattedField{Value: key, Warn: true}
}

func (f *Formatter) formatChainID(v ResolvedValue) FormattedField {
	if v.Kind != ResolvedInt {
		return FormattedField{Value: f.formatRaw(v), Warn: true}
	}
	id := v.Int.Uint64()
	if f.Helpers != nil {
		if name, ok := f.Helpers.NetworkName(id); ok {
			return FormattedField{Value: fmt.Sprintf("%s (id %d)", name, id)}
		}
	}
	return FormattedField{Value: fmt.Sprintf("chain id %d", id), Warn: true}
}

func (f *Formatter) formatNFTName(field Field, v ResolvedValue) FormattedField {
	id := ""
	if v.Kind == ResolvedInt {
		id = v.Int.String()
	} else {
		id = f.formatRaw(v)
	}
	collection, _ := f.resolveTokenParam(field, "collectionPath", "collection")
	if collection == "" {
		return FormattedField{Value: fmt.Sprintf("token #%s", id)}
	}
	label := collection
	if f.Helpers != nil {
		label = f.Helpers.ResolveAddress(collection, f.Resolver.Container.ChainID)
	}
	return FormattedField{Value: fmt.Sprintf("%s #%s", label, id)}
}

// ── group rendering ──────────────────────────────────────────────────────

func (f *Formatter) formatGroup(g Field) ([]FormattedField, error) {
	// A group with a path acts as a relative root for its children.
	var basePath Path
	if g.Path != "" {
		p, err := ParsePath(g.Path)
		if err != nil {
			return nil, err
		}
		basePath = p
	}

	// If the basePath points to an array (".[]"), we iterate.
	if strings.HasSuffix(g.Path, "[]") {
		// Strip the trailing [] so the base path resolves to the
		// array itself; we then iterate over its elements.
		stripped := strings.TrimSuffix(strings.TrimSuffix(g.Path, "[]"), ".")
		bp, _ := ParsePath(stripped)
		arr, err := f.Resolver.Resolve(bp)
		if err != nil {
			return nil, err
		}
		if arr.Kind != ResolvedArray {
			return nil, fmt.Errorf("erc7730: group path %s did not resolve to an array", g.Path)
		}
		out := []FormattedField{}
		for i, elem := range arr.Array {
			subResolver := *f.Resolver
			// Re-root the resolver's Data at this element so
			// child paths resolve against it.
			subResolver.Data = elem
			sub := &Formatter{Resolver: &subResolver, Helpers: f.Helpers}
			for _, child := range g.Fields {
				rows, err := sub.FormatField(child)
				if err != nil {
					continue
				}
				for _, r := range rows {
					r.Label = fmt.Sprintf("[%d] %s", i, r.Label)
					out = append(out, r)
				}
			}
		}
		return out, nil
	}

	// Non-iterating group: just nest children under basePath.
	out := []FormattedField{}
	for _, child := range g.Fields {
		if basePath.Original != "" && child.Path != "" {
			childPath, err := ParsePath(child.Path)
			if err == nil {
				joined := basePath.Join(childPath)
				child.Path = joined.String()
			}
		}
		rows, err := f.FormatField(child)
		if err != nil {
			continue
		}
		out = append(out, rows...)
	}
	return out, nil
}

// ── helpers ───────────────────────────────────────────────────────────────

func (f *Formatter) resolveTokenParam(field Field, pathKey, constKey string) (addr string, fromPath bool) {
	if v, ok := field.Params[pathKey].(string); ok && v != "" {
		p, err := ParsePath(v)
		if err == nil {
			r, err := f.Resolver.Resolve(p)
			if err == nil {
				return resolvedToAddressLike(r), true
			}
		}
	}
	if v, ok := field.Params[constKey].(string); ok {
		return v, false
	}
	return "", false
}

func resolvedToAddressLike(v ResolvedValue) string {
	switch v.Kind {
	case ResolvedAddress:
		return v.Addr
	case ResolvedString:
		return v.Str
	case ResolvedBytes:
		if len(v.Bytes) >= 20 {
			return "0x" + hex.EncodeToString(v.Bytes[:20])
		}
		return "0x" + hex.EncodeToString(v.Bytes)
	}
	return ""
}

func (f *Formatter) thresholdParam(field Field) (*big.Int, bool) {
	v, ok := field.Params["threshold"]
	if !ok {
		return nil, false
	}
	switch t := v.(type) {
	case string:
		n, _ := new(big.Int).SetString(strings.TrimPrefix(strings.TrimPrefix(t, "0x"), "0X"), 16)
		if n == nil {
			n, _ = new(big.Int).SetString(t, 10)
		}
		if n != nil {
			return n, true
		}
	case float64:
		return new(big.Int).SetInt64(int64(t)), true
	}
	return nil, false
}

func (f *Formatter) tokenInfo(addr string) (decimals uint64, symbol string, ok bool) {
	if f.Helpers == nil {
		return 0, "", false
	}
	chainID := f.Resolver.Container.ChainID
	return f.Helpers.TokenMetadata(addr, chainID)
}

func stringParam(field Field, key, fallback string) string {
	if v, ok := field.Params[key].(string); ok {
		return v
	}
	return fallback
}

func intParam(field Field, key string, fallback int) int {
	switch v := field.Params[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}

func (f *Formatter) nativeCurrencyAddresses(field Field) []string {
	if s := f.resolveParamRef(field.Params["nativeCurrencyAddress"]); s != "" {
		return []string{s}
	}
	switch v := field.Params["nativeCurrencyAddress"].(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func (f *Formatter) resolveParamRef(v any) string {
	s, ok := v.(string)
	if !ok || s == "" {
		return ""
	}
	if s[0] != '$' && s[0] != '#' && s[0] != '@' {
		return s
	}
	p, err := ParsePath(s)
	if err != nil {
		return ""
	}
	rv, err := f.Resolver.Resolve(p)
	if err != nil {
		return ""
	}
	switch rv.Kind {
	case ResolvedAddress:
		return rv.Addr
	case ResolvedString:
		return rv.Str
	default:
		return resolvedToAddressLike(rv)
	}
}

func containsCI(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.EqualFold(s, needle) {
			return true
		}
	}
	return false
}

func formatBigInt(n *big.Int) string {
	if n == nil {
		return "0"
	}
	return jarviscommon.BigToFloatString(n, 0)
}

// shouldShow applies the field's visibility rule against the resolved
// value. Returns (visible). MustMatch enforcement happens in render.go
// because a failed mustMatch is fatal to the whole clear-signed view.
func shouldShow(v Visibility, val ResolvedValue) bool {
	switch v.Kind {
	case VisibilityAlways:
		return true
	case VisibilityNever:
		return false
	case VisibilityOptional:
		return true
	case VisibilityIfNotIn:
		raw := rawStringForVisibility(val)
		for _, deny := range v.Values {
			if strings.EqualFold(deny, raw) {
				return false
			}
		}
		return true
	case VisibilityMustMatch:
		return false
	}
	return true
}

func rawStringForVisibility(v ResolvedValue) string {
	switch v.Kind {
	case ResolvedInt:
		return v.Int.String()
	case ResolvedString:
		return v.Str
	case ResolvedAddress:
		return v.Addr
	case ResolvedBool:
		return fmt.Sprintf("%t", v.Bool)
	case ResolvedBytes:
		return "0x" + hex.EncodeToString(v.Bytes)
	}
	return ""
}

// pathLabel turns a path into a label for cases where the descriptor
// doesn't provide one. "params.amountIn" → "amountIn".
func pathLabel(p Path) string {
	if len(p.Segments) == 0 {
		return p.Original
	}
	last := p.Segments[len(p.Segments)-1]
	if last.Kind == SegField {
		return last.Name
	}
	if len(p.Segments) >= 2 && p.Segments[len(p.Segments)-2].Kind == SegField {
		return p.Segments[len(p.Segments)-2].Name
	}
	return p.Original
}

// Visibility is a method on Field for convenience.
func (f Field) Visibility() Visibility { return f.Visible }
