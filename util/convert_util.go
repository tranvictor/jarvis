package util

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	. "github.com/tranvictor/jarvis/common"
	. "github.com/tranvictor/jarvis/networks"
)

func ConvertToBig(str string, network Network) (*big.Int, error) {
	str = strings.Trim(str, " ")
	parts := strings.Split(str, " ")
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid int format")
	}
	// in case there is no suffix
	if len(parts) == 1 {
		if len(str) > 2 && str[0:2] == "0x" {
			return hexutil.DecodeBig(str)
		} else {
			resultBig, ok := big.NewInt(0).SetString(str, 10)
			if !ok {
				return nil, fmt.Errorf("can't convert %s to big int", str)
			}
			return resultBig, nil
		}
	} else {
		floatStr := parts[0]

		tokenName := strings.Join(parts[1:], " ")
		if strings.ToLower(tokenName) == strings.ToLower(network.GetNativeTokenSymbol()) {
			return FloatStringToBig(floatStr, int64(network.GetNativeTokenDecimal()))
		}
		token, err := ConvertToAddress(fmt.Sprintf("%s token", tokenName))
		if err != nil {
			return nil, err
		}
		decimal, err := GetERC20Decimal(token.Hex(), network)
		if err != nil {
			return nil, err
		}
		return FloatStringToBig(floatStr, decimal)
	}
}

func ConvertToBool(str string) (bool, error) {
	str = strings.Trim(str, " ")
	if str == "true" {
		return true, nil
	}
	if str == "false" {
		return false, nil
	}
	return false, fmt.Errorf("bool value must be true|false")
}

func ConvertToAddress(str string) (common.Address, error) {
	str = strings.Trim(str, " ")
	if len(str) > 2 && str[0:2] == "0x" {
		addresses := ScanForAddresses(str)
		if len(addresses) == 0 {
			return common.Address{}, fmt.Errorf("invalid address")
		}
		if len(addresses) > 1 {
			return common.Address{}, fmt.Errorf("too many addresses provided")
		}
		return HexToAddress(addresses[0]), nil
	} else {
		addr, _, err := GetMatchingAddress(str)
		if err != nil {
			return common.Address{}, fmt.Errorf("address alias not found")
		}
		return HexToAddress(addr), nil
	}
}

func ConvertToHash(str string) (common.Hash, error) {
	str = strings.Trim(str, " ")
	if len(str) < 2 || str[0:2] != "0x" {
		return common.Hash{}, fmt.Errorf("hash must begin with 0x")
	}
	return HexToHash(str), nil
}

func ConvertToIntOrBig(str string, size int, network Network) (interface{}, error) {
	switch size {
	case 8, 16, 32, 64:
		return ConvertToInt(str, size)
	default:
		return ConvertToBig(str, network)
	}
}

func ConvertToUintOrBig(str string, size int, network Network) (interface{}, error) {
	switch size {
	case 8, 16, 32, 64:
		return ConvertToUint(str, size)
	default:
		return ConvertToBig(str, network)
	}
}

func ConvertParamStrToFixedByteType(name string, t abi.Type, strs []string, network Network) (interface{}, error) {
	switch t.Size {
	case 1:
		res := [][1]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [1]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 2:
		res := [][2]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [2]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 3:
		res := [][3]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [3]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 4:
		res := [][4]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [4]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 5:
		res := [][5]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [5]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 6:
		res := [][6]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [6]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 7:
		res := [][7]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [7]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 8:
		res := [][8]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [8]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 9:
		res := [][9]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [9]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 10:
		res := [][10]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [10]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 11:
		res := [][11]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [11]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 12:
		res := [][12]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [12]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 13:
		res := [][13]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [13]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 14:
		res := [][14]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [14]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 15:
		res := [][15]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [15]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 16:
		res := [][16]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [16]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 17:
		res := [][17]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [17]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 18:
		res := [][18]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [18]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 19:
		res := [][19]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [19]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 20:
		res := [][20]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [20]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 21:
		res := [][21]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [21]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 22:
		res := [][22]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [22]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 23:
		res := [][23]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [23]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 24:
		res := [][24]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [24]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 25:
		res := [][25]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [25]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 26:
		res := [][26]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [26]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 27:
		res := [][27]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [27]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 28:
		res := [][28]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [28]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 29:
		res := [][29]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [29]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 30:
		res := [][30]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [30]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 31:
		res := [][31]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [31]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	case 32:
		res := [][32]byte{}
		for _, str := range strs {
			tempBytes, err := ConvertToBytes(str)
			if err != nil {
				return res, err
			}
			realValue := [32]byte{}
			copy(realValue[:], tempBytes)
			res = append(res, realValue)
		}
		return res, nil
	}
	return []byte{}, fmt.Errorf("fixed byte array of size %d is not supported", t.Size)
}

func ConvertEthereumTypeToInputString(t abi.Type, value interface{}) (string, error) {
	return "", fmt.Errorf("not implmeneted")
}

func ConvertParamStrToTupleType(name string, t abi.Type, str string, network Network) (interface{}, error) {
	fmt.Printf("value str: %s\n", str)
	fmt.Printf("input name: %s\n", name)
	fmt.Printf("input type: %v\n", t)
	fmt.Printf("input tuple type: %v\n", t.TupleType)
	fmt.Printf("input tuple raw name: %v\n", t.TupleRawName)
	fmt.Printf("input tuple elems: %v\n", t.TupleElems)
	return nil, fmt.Errorf("not supported type: %s", t)
}

func ConvertParamStrToArray(name string, t abi.Type, str string, network Network) ([]interface{}, error) {
	// split to get the elements
	//   if element type is tuple or slice or array
	//       using regex to extract elements including []
	//   if not:
	//       if element is string:
	//           using regex to extract elements between ""
	//       if element is not string:
	//           using regex to extract elements split by ,
	// for each element:
	//    convert element to type
	// str = strings.Trim(str, " ")
	// if len(str) < 2 || str[0] != '[' || str[len(str)-1] != ']' {
	// 	return nil, fmt.Errorf("input must be wrapped by []")
	// }
	// if t.Elem.T == abi.TupleTy || t.Elem.T == abi.SliceTy || t.Elem.T == abi.ArrayTy {
	// } else if t.Elem.T == abi.StringTy {
	// } else {
	// }
	return nil, fmt.Errorf("unimplemented")
}

func ConvertParamStrToType(name string, t abi.Type, str string, network Network) (interface{}, error) {
	switch t.T {
	case abi.StringTy: // variable arrays are written at the end of the return bytes
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
		return ConvertToFixedBytes(str, t.Size)
	case abi.FunctionTy:
		return ConvertToBytes(str)
	case abi.TupleTy:
		return ConvertParamStrToTupleType(name, t, str, network)
	case abi.SliceTy, abi.ArrayTy:
		return ConvertParamStrToArray(name, t, str, network)
	// case abi.FixedPointTy:
	default:
		return nil, fmt.Errorf("not supported type: %s", t)
	}
}

func ConvertToUint(str string, size int) (interface{}, error) {
	str = strings.Trim(str, " ")
	if len(str) == 2 && str == "0x" {
		switch size {
		case 8:
			return uint8(0), nil
		case 16:
			return uint16(0), nil
		case 32:
			return uint32(0), nil
		case 64:
			return uint64(0), nil
		}
	}

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
	panic("unsupported int size")
}

func ConvertToInt(str string, size int) (interface{}, error) {
	str = strings.Trim(str, " ")
	if len(str) == 2 && str == "0x" {
		switch size {
		case 8:
			return int8(0), nil
		case 16:
			return int16(0), nil
		case 32:
			return int32(0), nil
		case 64:
			return int64(0), nil
		}
	}

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

func ConvertToFixedBytes(str string, size int) (interface{}, error) {
	bytes, err := ConvertToBytes(str)
	if err != nil {
		return []byte{}, err
	}
	switch size {
	case 1:
		res := [1]byte{}
		copy(res[:], bytes)
		return res, nil
	case 2:
		res := [2]byte{}
		copy(res[:], bytes)
		return res, nil
	case 3:
		res := [3]byte{}
		copy(res[:], bytes)
		return res, nil
	case 4:
		res := [4]byte{}
		copy(res[:], bytes)
		return res, nil
	case 5:
		res := [5]byte{}
		copy(res[:], bytes)
		return res, nil
	case 6:
		res := [6]byte{}
		copy(res[:], bytes)
		return res, nil
	case 7:
		res := [7]byte{}
		copy(res[:], bytes)
		return res, nil
	case 8:
		res := [8]byte{}
		copy(res[:], bytes)
		return res, nil
	case 9:
		res := [9]byte{}
		copy(res[:], bytes)
		return res, nil
	case 10:
		res := [10]byte{}
		copy(res[:], bytes)
		return res, nil
	case 11:
		res := [11]byte{}
		copy(res[:], bytes)
		return res, nil
	case 12:
		res := [12]byte{}
		copy(res[:], bytes)
		return res, nil
	case 13:
		res := [13]byte{}
		copy(res[:], bytes)
		return res, nil
	case 14:
		res := [14]byte{}
		copy(res[:], bytes)
		return res, nil
	case 15:
		res := [15]byte{}
		copy(res[:], bytes)
		return res, nil
	case 16:
		res := [16]byte{}
		copy(res[:], bytes)
		return res, nil
	case 17:
		res := [17]byte{}
		copy(res[:], bytes)
		return res, nil
	case 18:
		res := [18]byte{}
		copy(res[:], bytes)
		return res, nil
	case 19:
		res := [19]byte{}
		copy(res[:], bytes)
		return res, nil
	case 20:
		res := [20]byte{}
		copy(res[:], bytes)
		return res, nil
	case 21:
		res := [21]byte{}
		copy(res[:], bytes)
		return res, nil
	case 22:
		res := [22]byte{}
		copy(res[:], bytes)
		return res, nil
	case 23:
		res := [23]byte{}
		copy(res[:], bytes)
		return res, nil
	case 24:
		res := [24]byte{}
		copy(res[:], bytes)
		return res, nil
	case 25:
		res := [25]byte{}
		copy(res[:], bytes)
		return res, nil
	case 26:
		res := [26]byte{}
		copy(res[:], bytes)
		return res, nil
	case 27:
		res := [27]byte{}
		copy(res[:], bytes)
		return res, nil
	case 28:
		res := [28]byte{}
		copy(res[:], bytes)
		return res, nil
	case 29:
		res := [29]byte{}
		copy(res[:], bytes)
		return res, nil
	case 30:
		res := [30]byte{}
		copy(res[:], bytes)
		return res, nil
	case 31:
		res := [31]byte{}
		copy(res[:], bytes)
		return res, nil
	case 32:
		res := [32]byte{}
		copy(res[:], bytes)
		return res, nil
	}
	return []byte{}, fmt.Errorf("fixed byte array of size %d is not supported", size)
}

func ConvertToBytes(str string) ([]byte, error) {
	str = strings.Trim(str, " ")
	if len(str) == 2 && str == "0x" {
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
