package safe

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// builderValueToJarvisInput translates one contractInputsValues entry from
// the form the Safe UI writes into the form jarvis's own converters expect,
// so that util.ConvertParamStrToType — the exact same code path the
// interactive prompt uses — can do the actual conversion.
//
// The two formats agree on scalars but not on anything else:
//
//   - jarvis requires strings to be wrapped in double quotes
//     (util.ConvertToString errors otherwise); the Safe UI stores them bare.
//   - jarvis parses arrays/tuples with util.SplitArrayOrTupleStringInput,
//     which wants bare comma-separated elements in [] or (); the Safe UI
//     writes JSON, so elements arrive quoted.
//
// Rather than reimplement conversion, this walks the JSON value against the
// abi.Type and re-renders it in jarvis's syntax.
func builderValueToJarvisInput(t abi.Type, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)

	switch t.T {
	case abi.SliceTy, abi.ArrayTy, abi.TupleTy:
		v, err := decodeJSONValue(trimmed)
		if err != nil {
			// Not JSON — assume the operator already wrote jarvis syntax
			// (e.g. a hand-edited file using [0xaaa,0xbbb]) and pass it
			// through unchanged rather than failing on a form that works.
			return trimmed, nil
		}
		return renderJarvisValue(t, v)
	case abi.StringTy:
		// Always wrap: a value the operator genuinely wanted quoted round
		// trips correctly, because jarvis strips exactly one layer.
		return `"` + trimmed + `"`, nil
	default:
		return trimmed, nil
	}
}

// decodeJSONValue unmarshals into `any` with UseNumber so that uint256
// literals keep their exact decimal text instead of being routed through
// float64, which would silently corrupt anything above 2^53.
func decodeJSONValue(s string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// renderJarvisValue emits jarvis's input syntax for a decoded JSON value:
// arrays as [a,b], tuples as (a,b), strings quoted, everything else bare.
func renderJarvisValue(t abi.Type, v any) (string, error) {
	switch t.T {
	case abi.SliceTy, abi.ArrayTy:
		elems, ok := v.([]any)
		if !ok {
			return "", fmt.Errorf("expected a JSON array for %s", t.String())
		}
		if t.T == abi.ArrayTy && len(elems) != t.Size {
			return "", fmt.Errorf(
				"expected %d elements for %s, got %d", t.Size, t.String(), len(elems),
			)
		}
		parts := make([]string, 0, len(elems))
		for i, e := range elems {
			s, err := renderJarvisValue(*t.Elem, e)
			if err != nil {
				return "", fmt.Errorf("element %d: %w", i, err)
			}
			parts = append(parts, s)
		}
		return "[" + strings.Join(parts, ",") + "]", nil

	case abi.TupleTy:
		elems, err := tupleElems(t, v)
		if err != nil {
			return "", err
		}
		parts := make([]string, 0, len(elems))
		for i, e := range elems {
			s, err := renderJarvisValue(*t.TupleElems[i], e)
			if err != nil {
				name := ""
				if i < len(t.TupleRawNames) {
					name = t.TupleRawNames[i]
				}
				return "", fmt.Errorf("field %d (%s): %w", i, name, err)
			}
			parts = append(parts, s)
		}
		return "(" + strings.Join(parts, ",") + ")", nil

	case abi.StringTy:
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("expected a JSON string, got %T", v)
		}
		if err := checkNestedScalar(s); err != nil {
			return "", err
		}
		return `"` + s + `"`, nil

	case abi.BoolTy:
		switch b := v.(type) {
		case bool:
			if b {
				return "true", nil
			}
			return "false", nil
		case string:
			s := strings.TrimSpace(b)
			if s == "true" || s == "false" {
				return s, nil
			}
		}
		return "", fmt.Errorf("expected a boolean, got %T", v)

	default:
		// Numbers, addresses, bytes, hashes: jarvis takes these bare, so
		// the only work is turning the JSON scalar back into its text.
		s, err := scalarToString(v)
		if err != nil {
			return "", err
		}
		if err := checkNestedScalar(s); err != nil {
			return "", err
		}
		return s, nil
	}
}

// tupleElems accepts both shapes the Safe UI can produce for a tuple: a
// positional JSON array, or an object keyed by component name.
func tupleElems(t abi.Type, v any) ([]any, error) {
	switch tv := v.(type) {
	case []any:
		if len(tv) != len(t.TupleElems) {
			return nil, fmt.Errorf(
				"expected %d fields for %s, got %d", len(t.TupleElems), t.String(), len(tv),
			)
		}
		return tv, nil
	case map[string]any:
		if len(t.TupleRawNames) != len(t.TupleElems) {
			return nil, fmt.Errorf("tuple %s has unnamed fields; use a JSON array", t.String())
		}
		out := make([]any, 0, len(t.TupleElems))
		for _, name := range t.TupleRawNames {
			e, ok := tv[name]
			if !ok {
				return nil, fmt.Errorf("tuple %s is missing field %q", t.String(), name)
			}
			out = append(out, e)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected a JSON array or object for %s", t.String())
	}
}

func scalarToString(v any) (string, error) {
	switch s := v.(type) {
	case json.Number:
		return s.String(), nil
	case string:
		return strings.TrimSpace(s), nil
	case bool:
		if s {
			return "true", nil
		}
		return "false", nil
	case nil:
		return "", fmt.Errorf("value is null")
	default:
		return "", fmt.Errorf("unsupported JSON value %T", v)
	}
}

// checkNestedScalar rejects element values that jarvis's array/tuple splitter
// would mis-parse. util.SplitArrayOrTupleStringInput is not quote-aware and
// drops empty tokens, so a comma or bracket inside an element — or an empty
// element — would silently produce different calldata than the file
// describes. Refusing to encode is the only safe answer.
func checkNestedScalar(s string) error {
	if s == "" {
		return fmt.Errorf("empty values inside arrays/tuples are not supported")
	}
	if strings.ContainsAny(s, ",[]()") {
		return fmt.Errorf(
			"value %q contains one of ,[]() which jarvis cannot parse inside an array/tuple", s,
		)
	}
	return nil
}
