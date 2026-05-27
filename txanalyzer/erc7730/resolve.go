package erc7730

import (
	"fmt"
	"math/big"
	"strings"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

// ResolvedValue is the typed result of resolving a Path against the
// available data sources. Exactly one of the fields below is set,
// keyed by Kind. The renderer uses Kind to pick a default rendering
// when the descriptor doesn't specify a format.
type ResolvedValue struct {
	Kind  ResolvedKind
	Int   *big.Int // ResolvedInt
	Bytes []byte   // ResolvedBytes (also used for slices of strings)
	Str   string   // ResolvedString
	Bool  bool     // ResolvedBool
	Addr  string   // ResolvedAddress, normalised lower-case hex
	Tuple []ResolvedField // ResolvedTuple kind
	Array []ResolvedValue // ResolvedArray kind
	// Raw holds the *original* go value (from the ABI decode), used
	// for re-encoding paths that need to recurse further.
	Raw any
}

// ResolvedField pairs a tuple field's name with its value.
type ResolvedField struct {
	Name  string
	Value ResolvedValue
}

// ResolvedKind enumerates the shape of a resolved value.
type ResolvedKind int

const (
	ResolvedUnknown ResolvedKind = iota
	ResolvedInt
	ResolvedBytes
	ResolvedString
	ResolvedBool
	ResolvedAddress
	ResolvedTuple
	ResolvedArray
)

// Container holds the EVM transaction / EIP-712 container values that
// the @ path root references.
type Container struct {
	From    string // lower-case hex
	To      string // lower-case hex
	Value   *big.Int
	ChainID uint64
}

// Resolver looks up values referenced by Path under one of the four
// roots, against the decoded structured data (root `#`), the
// descriptor itself (`$`), or the container (`@`).
type Resolver struct {
	// Data is the decoded structured data root. For contract
	// calldata this is a ResolvedTuple wrapping each named parameter
	// of the matched function; for EIP-712 it is a ResolvedTuple
	// over the message's primary-type fields.
	Data ResolvedValue
	// Container is the EVM tx / EIP-712 envelope.
	Container Container
	// Descriptor is the parsed (merged) ERC-7730 file.
	Descriptor *Descriptor
}

// Resolve walks p from its root and returns the resolved leaf value.
func (r *Resolver) Resolve(p Path) (ResolvedValue, error) {
	var root ResolvedValue
	switch p.Root {
	case RootStruct:
		root = r.Data
	case RootContainer:
		root = r.containerValue()
	case RootDesc:
		return r.resolveDescriptor(p.Segments)
	}
	return walk(root, p.Segments)
}

func (r *Resolver) containerValue() ResolvedValue {
	fields := []ResolvedField{
		{"from", addrValue(r.Container.From)},
		{"to", addrValue(r.Container.To)},
		{"value", intValue(r.Container.Value)},
		{"chainId", intValue(new(big.Int).SetUint64(r.Container.ChainID))},
	}
	return ResolvedValue{Kind: ResolvedTuple, Tuple: fields}
}

func (r *Resolver) resolveDescriptor(segs []PathSeg) (ResolvedValue, error) {
	if r.Descriptor == nil || len(segs) == 0 {
		return ResolvedValue{}, fmt.Errorf("erc7730: cannot resolve $-path: no descriptor")
	}
	// Only the well-known prefixes are honoured: $.metadata.{constants,enums,maps,...}
	// and $.display.definitions.<name>. We expose them as opaque any
	// values; callers (formatters) deal with the typed shape.
	if segs[0].Kind != SegField {
		return ResolvedValue{}, fmt.Errorf("erc7730: $-path must start with a field name")
	}
	switch segs[0].Name {
	case "metadata":
		return r.resolveMetadata(segs[1:])
	case "display":
		return r.resolveDisplay(segs[1:])
	}
	return ResolvedValue{}, fmt.Errorf("erc7730: unknown $-path root %q", segs[0].Name)
}

func (r *Resolver) resolveMetadata(segs []PathSeg) (ResolvedValue, error) {
	if len(segs) < 2 {
		return ResolvedValue{}, fmt.Errorf("erc7730: $.metadata path needs at least one selector")
	}
	if segs[0].Kind != SegField {
		return ResolvedValue{}, fmt.Errorf("erc7730: $.metadata.<x> expected")
	}
	switch segs[0].Name {
	case "constants":
		v, ok := r.Descriptor.Metadata.Constants[segs[1].Name]
		if !ok {
			return ResolvedValue{}, fmt.Errorf("erc7730: constant %q not found", segs[1].Name)
		}
		return wrapAny(v), nil
	case "enums":
		e, ok := r.Descriptor.Metadata.Enums[segs[1].Name]
		if !ok {
			return ResolvedValue{}, fmt.Errorf("erc7730: enum %q not found", segs[1].Name)
		}
		// Surface enums as a string-keyed tuple for callers that
		// want to look up a specific key downstream.
		fields := make([]ResolvedField, 0, len(e))
		for k, v := range e {
			fields = append(fields, ResolvedField{Name: k, Value: stringValue(v)})
		}
		return ResolvedValue{Kind: ResolvedTuple, Tuple: fields, Raw: e}, nil
	case "maps":
		m, ok := r.Descriptor.Metadata.Maps[segs[1].Name]
		if !ok {
			return ResolvedValue{}, fmt.Errorf("erc7730: map %q not found", segs[1].Name)
		}
		return wrapAny(m), nil
	}
	return ResolvedValue{}, fmt.Errorf("erc7730: $.metadata.%s unsupported", segs[0].Name)
}

func (r *Resolver) resolveDisplay(segs []PathSeg) (ResolvedValue, error) {
	if len(segs) < 2 || segs[0].Kind != SegField || segs[0].Name != "definitions" {
		return ResolvedValue{}, fmt.Errorf("erc7730: only $.display.definitions.<name> supported")
	}
	def, ok := r.Descriptor.Display.Definitions[segs[1].Name]
	if !ok {
		return ResolvedValue{}, fmt.Errorf("erc7730: definition %q not found", segs[1].Name)
	}
	return wrapAny(def), nil
}

func walk(root ResolvedValue, segs []PathSeg) (ResolvedValue, error) {
	cur := root
	for i, seg := range segs {
		switch seg.Kind {
		case SegField:
			if cur.Kind != ResolvedTuple {
				return ResolvedValue{}, fmt.Errorf("erc7730: %q: expected tuple, got %s", seg.Name, kindName(cur.Kind))
			}
			found := false
			for _, f := range cur.Tuple {
				if f.Name == seg.Name {
					cur = f.Value
					found = true
					break
				}
			}
			if !found {
				return ResolvedValue{}, fmt.Errorf("erc7730: field %q not found", seg.Name)
			}
		case SegIndexAt:
			if cur.Kind != ResolvedArray && cur.Kind != ResolvedBytes && cur.Kind != ResolvedString {
				return ResolvedValue{}, fmt.Errorf("erc7730: index on non-array/bytes at %d", i)
			}
			switch cur.Kind {
			case ResolvedArray:
				idx := normalizeIndex(seg.Index, len(cur.Array))
				if idx < 0 || idx >= len(cur.Array) {
					return ResolvedValue{}, fmt.Errorf("erc7730: index %d out of range %d", seg.Index, len(cur.Array))
				}
				cur = cur.Array[idx]
			case ResolvedBytes:
				idx := normalizeIndex(seg.Index, len(cur.Bytes))
				if idx < 0 || idx >= len(cur.Bytes) {
					return ResolvedValue{}, fmt.Errorf("erc7730: byte index %d out of range %d", seg.Index, len(cur.Bytes))
				}
				cur = ResolvedValue{Kind: ResolvedBytes, Bytes: []byte{cur.Bytes[idx]}}
			}
		case SegSlice:
			cur = applySlice(cur, seg)
		}
	}
	return cur, nil
}

func applySlice(v ResolvedValue, s PathSeg) ResolvedValue {
	switch v.Kind {
	case ResolvedBytes:
		start, end := resolveSliceRange(len(v.Bytes), s)
		return ResolvedValue{Kind: ResolvedBytes, Bytes: v.Bytes[start:end]}
	case ResolvedString:
		start, end := resolveSliceRange(len(v.Str), s)
		return ResolvedValue{Kind: ResolvedString, Str: v.Str[start:end]}
	case ResolvedArray:
		start, end := resolveSliceRange(len(v.Array), s)
		return ResolvedValue{Kind: ResolvedArray, Array: v.Array[start:end]}
	}
	return v
}

func resolveSliceRange(length int, s PathSeg) (int, int) {
	start, end := 0, length
	if s.HasStart {
		start = s.Start
		if start < 0 {
			start += length
		}
	}
	if s.HasEnd {
		end = s.End
		if end < 0 {
			end += length
		}
	}
	if start < 0 {
		start = 0
	}
	if end > length {
		end = length
	}
	if end < start {
		end = start
	}
	return start, end
}

func normalizeIndex(idx, length int) int {
	if idx < 0 {
		return length + idx
	}
	return idx
}

// IsArrayElementPath returns true when path ends in `.[]` (the
// "iterate over all elements" suffix used by fields[].fields and by
// parameter-array references).
func IsArrayElementPath(s string) bool {
	return strings.HasSuffix(s, "[]") || strings.HasSuffix(s, "[].")
}

// ── value constructors used by the structured-data builder ──────────────────

func intValue(b *big.Int) ResolvedValue {
	if b == nil {
		b = new(big.Int)
	}
	return ResolvedValue{Kind: ResolvedInt, Int: new(big.Int).Set(b), Raw: b}
}

func stringValue(s string) ResolvedValue {
	return ResolvedValue{Kind: ResolvedString, Str: s, Raw: s}
}

func boolValue(v bool) ResolvedValue { return ResolvedValue{Kind: ResolvedBool, Bool: v, Raw: v} }

func bytesValue(b []byte) ResolvedValue {
	return ResolvedValue{Kind: ResolvedBytes, Bytes: append([]byte(nil), b...), Raw: b}
}

func addrValue(s string) ResolvedValue {
	return ResolvedValue{Kind: ResolvedAddress, Addr: strings.ToLower(s), Str: s, Raw: s}
}

// wrapAny wraps a Go scalar/string/map into a best-effort
// ResolvedValue. Used for $-root paths where the underlying data is
// already opaque to the resolver.
func wrapAny(v any) ResolvedValue {
	switch t := v.(type) {
	case string:
		return stringValue(t)
	case bool:
		return boolValue(t)
	case int:
		return intValue(big.NewInt(int64(t)))
	case int64:
		return intValue(big.NewInt(t))
	case float64:
		return intValue(big.NewInt(int64(t)))
	case uint64:
		return intValue(new(big.Int).SetUint64(t))
	case *big.Int:
		return intValue(t)
	case []byte:
		return bytesValue(t)
	}
	return ResolvedValue{Raw: v}
}

func kindName(k ResolvedKind) string {
	switch k {
	case ResolvedInt:
		return "int"
	case ResolvedBytes:
		return "bytes"
	case ResolvedString:
		return "string"
	case ResolvedBool:
		return "bool"
	case ResolvedAddress:
		return "address"
	case ResolvedTuple:
		return "tuple"
	case ResolvedArray:
		return "array"
	}
	return "unknown"
}

// ── conversion from jarvis ParamResult into the resolver tree ────────────

// BuildDataFromParams converts a jarvis ParamResult slice (as produced
// by txanalyzer.AnalyzeFunctionCallRecursively) into the
// ResolvedTuple shape the path resolver expects. The keys come from
// param names, which must align with the Solidity argument names used
// in the descriptor's format key (the spec mandates this alignment).
func BuildDataFromParams(params []jarviscommon.ParamResult) ResolvedValue {
	tuple := []ResolvedField{}
	for _, p := range params {
		tuple = append(tuple, ResolvedField{Name: p.Name, Value: paramToResolved(p)})
	}
	return ResolvedValue{Kind: ResolvedTuple, Tuple: tuple}
}

func paramToResolved(p jarviscommon.ParamResult) ResolvedValue {
	switch {
	case p.Values != nil && (len(p.Values) > 1 || isArrayType(p.Type)):
		arr := make([]ResolvedValue, len(p.Values))
		for i, v := range p.Values {
			arr[i] = valueToResolved(v, scalarTypeOf(p.Type))
		}
		return ResolvedValue{Kind: ResolvedArray, Array: arr}
	case p.Values != nil:
		return valueToResolved(p.Values[0], p.Type)
	case p.Tuples != nil:
		if len(p.Tuples) == 1 && !isArrayType(p.Type) {
			tuple := []ResolvedField{}
			for _, f := range p.Tuples[0].Values {
				tuple = append(tuple, ResolvedField{Name: f.Name, Value: paramToResolved(f)})
			}
			return ResolvedValue{Kind: ResolvedTuple, Tuple: tuple}
		}
		arr := make([]ResolvedValue, 0, len(p.Tuples))
		for _, tup := range p.Tuples {
			fields := []ResolvedField{}
			for _, f := range tup.Values {
				fields = append(fields, ResolvedField{Name: f.Name, Value: paramToResolved(f)})
			}
			arr = append(arr, ResolvedValue{Kind: ResolvedTuple, Tuple: fields})
		}
		return ResolvedValue{Kind: ResolvedArray, Array: arr}
	case p.Arrays != nil:
		arr := make([]ResolvedValue, 0, len(p.Arrays))
		for _, inner := range p.Arrays {
			arr = append(arr, paramToResolved(inner))
		}
		return ResolvedValue{Kind: ResolvedArray, Array: arr}
	}
	return ResolvedValue{}
}

func valueToResolved(v jarviscommon.Value, abiType string) ResolvedValue {
	switch v.Kind {
	case jarviscommon.DisplayAddress:
		return addrValue(v.Raw)
	case jarviscommon.DisplayInteger, jarviscommon.DisplayToken:
		n, _ := new(big.Int).SetString(v.Raw, 10)
		if n == nil {
			n = new(big.Int)
		}
		return intValue(n)
	}
	// Try to be clever about the underlying ABI type for raw values:
	// hex strings starting with 0x map to bytes, "true/false" to bool,
	// everything else to string.
	switch {
	case strings.HasPrefix(abiType, "bool"):
		return boolValue(v.Raw == "true")
	case strings.HasPrefix(abiType, "bytes"):
		b, _ := hexToBytes(v.Raw)
		return bytesValue(b)
	case strings.HasPrefix(abiType, "uint") || strings.HasPrefix(abiType, "int"):
		n, _ := new(big.Int).SetString(strings.TrimPrefix(v.Raw, "0x"), 0)
		if n == nil {
			n = new(big.Int)
		}
		return intValue(n)
	case strings.HasPrefix(abiType, "string"):
		return stringValue(v.Raw)
	}
	return stringValue(v.Raw)
}

func isArrayType(t string) bool { return strings.HasSuffix(t, "]") || strings.Contains(t, "[]") }

// scalarTypeOf strips one trailing array suffix from a Solidity type:
// "uint256[]" → "uint256", "address[3]" → "address", "bool" → "bool".
func scalarTypeOf(t string) string {
	if i := strings.LastIndex(t, "["); i >= 0 {
		return t[:i]
	}
	return t
}

func hexToBytes(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if len(s)%2 == 1 {
		s = "0" + s
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(s)/2; i++ {
		b, err := strconvParseByte(s[2*i : 2*i+2])
		if err != nil {
			return nil, err
		}
		out[i] = b
	}
	return out, nil
}

func strconvParseByte(s string) (byte, error) {
	var b byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		var nibble byte
		switch {
		case c >= '0' && c <= '9':
			nibble = c - '0'
		case c >= 'a' && c <= 'f':
			nibble = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			nibble = c - 'A' + 10
		default:
			return 0, fmt.Errorf("invalid hex char %q", c)
		}
		b = b<<4 | nibble
	}
	return b, nil
}
