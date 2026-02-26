package util

import (
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	jarviscommon "github.com/tranvictor/jarvis/common"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
)

func ConvertToBig(str string, network jarvisnetworks.Network) (*big.Int, error) {
	str = strings.Trim(str, " ")
	if str == "" {
		return nil, fmt.Errorf("invalid int format: empty string")
	}
	parts := strings.Split(str, " ")
	// Single token — either hex or decimal integer.
	if len(parts) == 1 {
		if strings.HasPrefix(str, "0x") {
			return hexutil.DecodeBig(str)
		}
		resultBig, ok := big.NewInt(0).SetString(str, 10)
		if !ok {
			return nil, fmt.Errorf("can't convert %s to big int", str)
		}
		return resultBig, nil
	}
	// Two-token form: "1.5 KNC" or "1.5 ETH".
	floatStr := parts[0]
	tokenName := strings.Join(parts[1:], " ")
	if strings.EqualFold(tokenName, network.GetNativeTokenSymbol()) {
		return jarviscommon.FloatStringToBig(floatStr, network.GetNativeTokenDecimal())
	}
	token, err := ConvertToAddress(fmt.Sprintf("%s token", tokenName))
	if err != nil {
		return nil, err
	}
	decimal, err := GetERC20Decimal(token.Hex(), network)
	if err != nil {
		return nil, err
	}
	return jarviscommon.FloatStringToBig(floatStr, decimal)
}

func ConvertToBool(str string) (bool, error) {
	switch strings.Trim(str, " ") {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf(`bool value must be "true" or "false"`)
}

func ConvertToAddress(str string) (common.Address, error) {
	str = strings.Trim(str, " ")
	if strings.HasPrefix(str, "0x") {
		addresses := ScanForAddresses(str)
		if len(addresses) == 0 {
			return common.Address{}, fmt.Errorf("invalid address")
		}
		if len(addresses) > 1 {
			return common.Address{}, fmt.Errorf("too many addresses provided")
		}
		return jarviscommon.HexToAddress(addresses[0]), nil
	}
	addr, _, err := GetMatchingAddress(str)
	if err != nil {
		return common.Address{}, fmt.Errorf("address alias not found")
	}
	return jarviscommon.HexToAddress(addr), nil
}

func ConvertToHash(str string) (common.Hash, error) {
	str = strings.Trim(str, " ")
	if !strings.HasPrefix(str, "0x") {
		return common.Hash{}, fmt.Errorf("hash must begin with 0x")
	}
	return jarviscommon.HexToHash(str), nil
}

func ConvertToIntOrBig(str string, size int, network jarvisnetworks.Network) (interface{}, error) {
	switch size {
	case 8, 16, 32, 64:
		return ConvertToInt(str, size)
	default:
		return ConvertToBig(str, network)
	}
}

func ConvertToUintOrBig(str string, size int, network jarvisnetworks.Network) (interface{}, error) {
	switch size {
	case 8, 16, 32, 64:
		return ConvertToUint(str, size)
	default:
		return ConvertToBig(str, network)
	}
}

// ConvertParamStrToFixedByteType converts a slice of hex strings into a typed
// slice of [N]byte arrays. The exact array type (e.g. [32]byte) is derived
// from t.GetType() so no 32-case switch is needed.
func ConvertParamStrToFixedByteType(t abi.Type, strs []string) (interface{}, error) {
	arrType := t.GetType() // reflect.Type for [N]byte
	res := reflect.MakeSlice(reflect.SliceOf(arrType), 0, len(strs))
	for _, str := range strs {
		raw, err := ConvertToBytes(str)
		if err != nil {
			return nil, err
		}
		arr := reflect.New(arrType).Elem()
		reflect.Copy(arr, reflect.ValueOf(raw))
		res = reflect.Append(res, arr)
	}
	return res.Interface(), nil
}

func ConvertEthereumTypeToInputString(t abi.Type, value interface{}) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func ConvertParamStrToTupleType(
	name string,
	t abi.Type,
	str string,
	network jarvisnetworks.Network,
) (interface{}, error) {
	jarviscommon.DebugPrintf("value str: %s\n", str)
	jarviscommon.DebugPrintf("input name: %s\n", name)
	jarviscommon.DebugPrintf("input type: %v\n", t)
	jarviscommon.DebugPrintf("input tuple type: %v\n", t.TupleType)
	jarviscommon.DebugPrintf("input tuple raw name: %v\n", t.TupleRawName)
	jarviscommon.DebugPrintf("input tuple elems: %v\n", t.TupleElems)

	inputElems, err := SplitArrayOrTupleStringInput(str)
	if err != nil {
		return nil, err
	}
	if len(inputElems) != t.TupleType.NumField() {
		return nil, fmt.Errorf("your input doesn't have enough field for the tuple")
	}

	tupleInstance := reflect.New(t.TupleType).Elem()
	for i := 0; i < t.TupleType.NumField(); i++ {
		field := t.TupleType.Field(i)
		jarviscommon.DebugPrintf("Input for field %s (%s): %s\n", field.Name, field.Type, inputElems[i])

		fieldName := ""
		if t.TupleElems[i].T == abi.AddressTy {
			// TODO: handle token-name resolution for address-typed tuple fields
		}

		value, err := ConvertParamStrToType(fieldName, *t.TupleElems[i], inputElems[i], network)
		if err != nil {
			return nil, fmt.Errorf(
				"couldn't parse field %d (%s), index %d with input \"%s\": %w",
				i, field.Name, field.Type, inputElems[i], err)
		}

		jarviscommon.DebugPrintf("parsed value for field %s: %+v\n", field.Name, value)
		tupleInstance.FieldByIndex([]int{i}).Set(reflect.ValueOf(value))
	}

	return tupleInstance.Interface(), nil
}

func ConvertParamStrToArray(
	name string,
	t abi.Type,
	str string,
	network jarvisnetworks.Network,
) (interface{}, error) {
	jarviscommon.DebugPrintf("value str: %s\n", str)
	jarviscommon.DebugPrintf("input name: %s\n", name)
	jarviscommon.DebugPrintf("input type: %v\n", t)
	jarviscommon.DebugPrintf("input array elem type: %v\n", t.Elem)

	inputElems, err := SplitArrayOrTupleStringInput(str)
	if err != nil {
		return nil, err
	}

	arrayInstance := reflect.MakeSlice(reflect.SliceOf(t.Elem.GetType()), 0, len(inputElems))
	for i, elemStr := range inputElems {
		jarviscommon.DebugPrintf("Input for element %dth of type %+v: %s\n", i, t.Elem, elemStr)

		elemName := fmt.Sprintf("%s[%d]", name, i)
		value, err := ConvertParamStrToType(elemName, *t.Elem, elemStr, network)
		if err != nil {
			return nil, fmt.Errorf(
				"couldn't parse element %dth (%s) with input \"%s\": %w",
				i, t.Elem, elemStr, err)
		}

		jarviscommon.DebugPrintf("parsed value for element %dth: %+v\n", i, value)
		arrayInstance = reflect.Append(arrayInstance, reflect.ValueOf(value))
	}

	return arrayInstance.Interface(), nil
}

func ConvertParamStrToType(
	name string,
	t abi.Type,
	str string,
	network jarvisnetworks.Network,
) (any, error) {
	switch t.T {
	case abi.StringTy:
		return ConvertToString(str)
	case abi.IntTy:
		return ConvertToIntOrBig(str, t.Size, network)
	case abi.UintTy:
		return ConvertToUintOrBig(str, t.Size, network)
	case abi.BoolTy:
		return ConvertToBool(str)
	case abi.AddressTy:
		lcName := strings.ToLower(name)
		if lcName == "token" || lcName == "tokens" || lcName == "asset" {
			return ConvertToAddress(fmt.Sprintf("%s token", str))
		}
		return ConvertToAddress(str)
	case abi.HashTy:
		return ConvertToHash(str)
	case abi.BytesTy:
		return ConvertToBytes(str)
	case abi.FixedBytesTy:
		return ConvertToFixedBytes(str, t)
	case abi.FunctionTy:
		return ConvertToBytes(str)
	case abi.TupleTy:
		return ConvertParamStrToTupleType(name, t, str, network)
	case abi.SliceTy, abi.ArrayTy:
		return ConvertParamStrToArray(name, t, str, network)
	default:
		return nil, fmt.Errorf("not supported type: %s", t)
	}
}

func ConvertToUint(str string, size int) (interface{}, error) {
	str = strings.Trim(str, " ")
	switch size {
	case 8:
		res, err := strconv.ParseUint(str, 0, 8)
		return uint8(res), err
	case 16:
		res, err := strconv.ParseUint(str, 0, 16)
		return uint16(res), err
	case 32:
		res, err := strconv.ParseUint(str, 0, 32)
		return uint32(res), err
	case 64:
		res, err := strconv.ParseUint(str, 0, 64)
		return uint64(res), err
	}
	panic("unsupported uint size")
}

func ConvertToInt(str string, size int) (interface{}, error) {
	str = strings.Trim(str, " ")
	switch size {
	case 8:
		res, err := strconv.ParseInt(str, 0, 8)
		return int8(res), err
	case 16:
		res, err := strconv.ParseInt(str, 0, 16)
		return int16(res), err
	case 32:
		res, err := strconv.ParseInt(str, 0, 32)
		return int32(res), err
	case 64:
		res, err := strconv.ParseInt(str, 0, 64)
		return int64(res), err
	}
	panic("unsupported int size")
}

// ConvertToFixedBytes converts a hex string into a [N]byte array. The exact
// array type is derived from t.GetType() so no 32-case switch is needed.
func ConvertToFixedBytes(str string, t abi.Type) (interface{}, error) {
	raw, err := ConvertToBytes(str)
	if err != nil {
		return nil, err
	}
	arrType := t.GetType() // reflect.Type for [N]byte
	arr := reflect.New(arrType).Elem()
	reflect.Copy(arr, reflect.ValueOf(raw))
	return arr.Interface(), nil
}

func ConvertToBytes(str string) ([]byte, error) {
	str = strings.Trim(str, " ")
	if str == "0x" {
		return []byte{}, nil
	}
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		return []byte(str[1 : len(str)-1]), nil
	}
	return hexutil.Decode(str)
}

func ConvertToString(str string) (string, error) {
	str = strings.Trim(str, " ")
	if len(str) < 2 || str[0] != '"' || str[len(str)-1] != '"' {
		return "", fmt.Errorf(`string must be wrapped by ""`)
	}
	return str[1 : len(str)-1], nil
}
