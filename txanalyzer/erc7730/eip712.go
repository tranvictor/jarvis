package erc7730

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// buildDataFromTypedData converts an EIP-712 message body into the
// resolver's ResolvedTuple shape. We walk the primaryType's fields
// using the typed-data Types map to recurse into nested struct
// references; arrays and primitives map directly.
//
// `keyNames` is the parameter-name list parsed from the descriptor's
// `display.formats` key. When the typed-data primaryType fields and
// the descriptor's names disagree (rare, usually a typo), we still
// produce a tuple using the typed-data names — descriptors that
// reference unknown names will surface their own resolver errors.
func buildDataFromTypedData(td *apitypes.TypedData, keyNames []string) ResolvedValue {
	_ = keyNames
	root := td.Message
	return buildTypedValue(td, td.PrimaryType, root)
}

func buildTypedValue(td *apitypes.TypedData, primaryType string, val any) ResolvedValue {
	switch v := val.(type) {
	case map[string]any:
		fields, ok := td.Types[primaryType]
		if !ok {
			return ResolvedValue{}
		}
		out := ResolvedValue{Kind: ResolvedTuple}
		for _, f := range fields {
			out.Tuple = append(out.Tuple, ResolvedField{
				Name:  f.Name,
				Value: buildTypedValue(td, f.Type, v[f.Name]),
			})
		}
		return out
	case []any:
		// Arrays: element type is the array's element type — strip
		// one trailing "[]" / "[N]".
		elemType := scalarTypeOf(primaryType)
		arr := make([]ResolvedValue, len(v))
		for i, e := range v {
			arr[i] = buildTypedValue(td, elemType, e)
		}
		return ResolvedValue{Kind: ResolvedArray, Array: arr}
	}
	// Primitive: try to coerce to the appropriate ResolvedKind based
	// on the declared Solidity type.
	return coerceTypedPrimitive(primaryType, val)
}

func coerceTypedPrimitive(soltype string, val any) ResolvedValue {
	switch {
	case soltype == "address":
		return addrValue(fmt.Sprintf("%v", val))
	case strings.HasPrefix(soltype, "bool"):
		if b, ok := val.(bool); ok {
			return boolValue(b)
		}
		return boolValue(false)
	case strings.HasPrefix(soltype, "bytes"):
		switch t := val.(type) {
		case string:
			b, err := hexDecode(t)
			if err == nil {
				return bytesValue(b)
			}
			return stringValue(t)
		case []byte:
			return bytesValue(t)
		}
	case strings.HasPrefix(soltype, "string"):
		if s, ok := val.(string); ok {
			return stringValue(s)
		}
		return stringValue(fmt.Sprintf("%v", val))
	case strings.HasPrefix(soltype, "uint"), strings.HasPrefix(soltype, "int"):
		return intValue(toBigInt(val))
	}
	return stringValue(fmt.Sprintf("%v", val))
}

func toBigInt(v any) *big.Int {
	switch t := v.(type) {
	case string:
		// Numeric strings: try base-10 then base-16.
		n, ok := new(big.Int).SetString(t, 10)
		if ok {
			return n
		}
		n, ok = new(big.Int).SetString(strings.TrimPrefix(strings.TrimPrefix(t, "0x"), "0X"), 16)
		if ok {
			return n
		}
		return new(big.Int)
	case float64:
		return new(big.Int).SetInt64(int64(t))
	case int:
		return new(big.Int).SetInt64(int64(t))
	case int64:
		return new(big.Int).SetInt64(t)
	case uint64:
		return new(big.Int).SetUint64(t)
	case *big.Int:
		return new(big.Int).Set(t)
	case []byte:
		return new(big.Int).SetBytes(t)
	}
	return new(big.Int)
}

// Suppress an unused-import warning if hex ends up unused after
// downstream edits. Currently consumed by hexDecode in match.go.
var _ = hex.EncodeToString
