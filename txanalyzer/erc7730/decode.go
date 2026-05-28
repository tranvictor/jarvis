package erc7730

import (
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// buildDataFromCalldata unpacks calldata with the matched format's ABI
// shape and builds the # root the descriptor paths reference. This
// mirrors go-ethereum's struct field binding (same as jarvis's
// txanalyzer) and avoids subtle mismatches from the ParamResult
// conversion layer.
func buildDataFromCalldata(matched *MatchedFormat, calldata []byte, method *abi.Method) (ResolvedValue, error) {
	if len(calldata) < 4 {
		return ResolvedValue{}, fmt.Errorf("erc7730: calldata too short")
	}
	values, err := method.Inputs.UnpackValues(calldata[4:])
	if err != nil {
		return ResolvedValue{}, err
	}
	fields := make([]ResolvedField, len(method.Inputs))
	for i, input := range method.Inputs {
		name := input.Name
		if name == "" && i < len(matched.ParamNames) {
			name = matched.ParamNames[i]
		}
		fields[i] = ResolvedField{
			Name:  name,
			Value: abiValueToResolved(input.Type, values[i]),
		}
	}
	return ResolvedValue{Kind: ResolvedTuple, Tuple: fields}, nil
}

// abiValueToResolved converts one ABI-decoded Go value into the
// resolver tree. Tuple structs from go-ethereum are walked via
// reflection using the type's TupleRawNames — the same field-name
// mapping txanalyzer uses.
func abiValueToResolved(t abi.Type, v any) ResolvedValue {
	if v == nil {
		return ResolvedValue{}
	}
	switch t.T {
	case abi.AddressTy:
		switch a := v.(type) {
		case common.Address:
			return addrValue(a.Hex())
		case []byte:
			if len(a) >= 20 {
				return addrValue(common.BytesToAddress(a).Hex())
			}
		}
	case abi.UintTy, abi.IntTy:
		return intValue(asBigInt(v))
	case abi.BoolTy:
		b, _ := v.(bool)
		return boolValue(b)
	case abi.StringTy:
		s, _ := v.(string)
		return stringValue(s)
	case abi.BytesTy, abi.FixedBytesTy:
		switch b := v.(type) {
		case []byte:
			return bytesValue(b)
		}
	case abi.SliceTy, abi.ArrayTy:
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Pointer {
			rv = rv.Elem()
		}
		out := make([]ResolvedValue, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = abiValueToResolved(*t.Elem, rv.Index(i).Interface())
		}
		return ResolvedValue{Kind: ResolvedArray, Array: out}
	case abi.TupleTy:
		return abiTupleToResolved(t, v)
	}
	return wrapAny(v)
}

func abiTupleToResolved(t abi.Type, v any) ResolvedValue {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	caser := cases.Title(language.Und, cases.NoLower)
	fields := make([]ResolvedField, len(t.TupleElems))
	for i, elem := range t.TupleElems {
		name := t.TupleRawNames[i]
		fv := rv.FieldByName(caser.String(name))
		if !fv.IsValid() {
			// Anonymous or compiler-renamed field — fall back to index.
			if i < rv.NumField() {
				fv = rv.Field(i)
			}
		}
		var val any
		if fv.IsValid() {
			val = fv.Interface()
		}
		fields[i] = ResolvedField{Name: name, Value: abiValueToResolved(*elem, val)}
	}
	return ResolvedValue{Kind: ResolvedTuple, Tuple: fields}
}

func asBigInt(v any) *big.Int {
	switch n := v.(type) {
	case *big.Int:
		if n == nil {
			return new(big.Int)
		}
		return new(big.Int).Set(n)
	case uint64:
		return new(big.Int).SetUint64(n)
	case int64:
		return big.NewInt(n)
	case uint:
		return new(big.Int).SetUint64(uint64(n))
	case int:
		return big.NewInt(int64(n))
	default:
		if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && !rv.IsNil() {
			return asBigInt(rv.Elem().Interface())
		}
		// Last resort: parse decimal string forms.
		if s, ok := v.(string); ok {
			if bi, ok := new(big.Int).SetString(strings.TrimPrefix(s, "0x"), 0); ok {
				return bi
			}
			if bi, ok := new(big.Int).SetString(s, 10); ok {
				return bi
			}
		}
	}
	return new(big.Int)
}
