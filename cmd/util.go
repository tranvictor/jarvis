package cmd

import (
	"bufio"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/db"
	"github.com/tranvictor/jarvis/util"
)

const (
	NEXT int = -1
	BACK int = -2
)

func promptIndex(prompter string, min, max int) int {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s: ", prompter)
		text, _ := reader.ReadString('\n')
		indexInput := strings.Trim(text[0:len(text)-1], "\r\n")
		if indexInput == "next" {
			return NEXT
		} else if indexInput == "back" {
			return BACK
		} else {
			index, err := strconv.Atoi(indexInput)
			if err != nil {
				fmt.Printf("Jarvis: Please enter the index or 'next' or 'back'\n")
			} else if min <= index && index <= max {
				return index
			} else {
				fmt.Printf("Jarvis: Please enter the index. It should be any number from %d-%d\n", min, max)
			}
		}
	}
}

func promptInput(prompter string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s: ", prompter)
	text, _ := reader.ReadString('\n')
	return strings.Trim(text[0:len(text)-1], "\r\n")
}

func promptFilePath(prompter string) string {
	return promptInput(prompter)
}

func promptParam(input abi.Argument, prefill string) (interface{}, error) {
	t := input.Type
	switch t.T {
	case abi.SliceTy, abi.ArrayTy:
		return promptArray(input, prefill)
	default:
		return promptNonArray(input, prefill)
	}
}

func convertToFixedBytes(str string, size int) (interface{}, error) {
	str = strings.Trim(str, " ")
	if len(str) == 2 && str == "0x" {
		return []byte{}, nil
	}
	bytes, err := hexutil.Decode(str)
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

func convertToBytes(str string) ([]byte, error) {
	str = strings.Trim(str, " ")
	if len(str) == 2 && str == "0x" {
		return []byte{}, nil
	}
	return hexutil.Decode(str)
}

func convertToString(str string) (string, error) {
	str = strings.Trim(str, " ")
	if len(str) < 2 || str[0] != '"' || str[len(str)-1] != '"' {
		return "", fmt.Errorf(`string must be wrapped by ""`)
	}
	return str[1 : len(str)-1], nil
}

func convertToBig(str string) (*big.Int, error) {
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
		floatNum, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return nil, err
		}
		tokenName := strings.Join(parts[1:], " ")
		if strings.ToLower(tokenName) == "eth" {
			return ethutils.FloatToBigInt(floatNum, 18), nil
		}
		token, err := convertToAddress(fmt.Sprintf("%s token", tokenName))
		if err != nil {
			return nil, err
		}
		decimal, err := util.GetERC20Decimal(token.Hex(), config.Network)
		if err != nil {
			return nil, err
		}
		return ethutils.FloatToBigInt(floatNum, decimal), nil
	}
}

func convertToBool(str string) (bool, error) {
	str = strings.Trim(str, " ")
	if str == "true" {
		return true, nil
	}
	if str == "false" {
		return false, nil
	}
	return false, fmt.Errorf("bool value must be true|false")
}

func convertToAddress(str string) (common.Address, error) {
	str = strings.Trim(str, " ")
	if len(str) > 2 && str[0:2] == "0x" {
		addresses := util.ScanForAddresses(str)
		if len(addresses) == 0 {
			return common.Address{}, fmt.Errorf("invalid address")
		}
		if len(addresses) > 1 {
			return common.Address{}, fmt.Errorf("too many addresses provided")
		}
		return ethutils.HexToAddress(addresses[0]), nil
	} else {
		addr, err := db.GetAddress(str)
		if err != nil {
			return common.Address{}, fmt.Errorf("address alias not found")
		}
		return ethutils.HexToAddress(addr.Address), nil
	}
}

func convertToHash(str string) (common.Hash, error) {
	str = strings.Trim(str, " ")
	if len(str) < 2 || str[0:2] != "0x" {
		return common.Hash{}, fmt.Errorf("hash must begin with 0x")
	}
	return ethutils.HexToHash(str), nil
}

func convertParamStrToType(name string, t abi.Type, str string) (interface{}, error) {
	switch t.T {
	case abi.StringTy: // variable arrays are written at the end of the return bytes
		return convertToString(str)
	case abi.IntTy, abi.UintTy:
		return convertToBig(str)
	case abi.BoolTy:
		return convertToBool(str)
	case abi.AddressTy:
		if strings.ToLower(name) == "token" || strings.ToLower(name) == "asset" {
			return convertToAddress(fmt.Sprintf("%s token", str))
		}
		return convertToAddress(str)
	case abi.HashTy:
		return convertToHash(str)
	case abi.BytesTy:
		return convertToBytes(str)
	case abi.FixedBytesTy:
		fmt.Printf("fixed bytes type with size: %d\n", t.Size)
		return convertToFixedBytes(str, t.Size)
	case abi.FunctionTy:
		return convertToBytes(str)
	default:
		return nil, fmt.Errorf("not supported type: %s", t)
	}
}

func promptArray(input abi.Argument, prefill string) (interface{}, error) {
	var inpStr string
	if prefill == "" {
		inpStr = promptInput("")
	} else {
		inpStr = prefill
	}
	inpStr = strings.Trim(inpStr, " ")
	if len(inpStr) < 2 || inpStr[0] != '[' || inpStr[len(inpStr)-1] != ']' {
		return nil, fmt.Errorf("input must be wrapped by []")
	}
	arrayContent := strings.Trim(inpStr[1:len(inpStr)-1], " ")
	paramsStr := strings.Split(arrayContent, ",")

	switch input.Type.Elem.T {
	case abi.StringTy: // variable arrays are written at the end of the return bytes
		result := []string{}
		if len(arrayContent) == 0 {
			return result, nil
		}
		for _, p := range paramsStr {
			converted, err := convertParamStrToType(input.Name, *input.Type.Elem, p)
			if err != nil {
				return nil, err
			}
			result = append(result, converted.(string))
		}
		return result, nil
	case abi.IntTy, abi.UintTy:
		result := []*big.Int{}
		if len(arrayContent) == 0 {
			return result, nil
		}
		for _, p := range paramsStr {
			converted, err := convertParamStrToType(input.Name, *input.Type.Elem, p)
			if err != nil {
				return nil, err
			}
			result = append(result, converted.(*big.Int))
		}
		return result, nil
	case abi.BoolTy:
		result := []bool{}
		if len(arrayContent) == 0 {
			return result, nil
		}
		for _, p := range paramsStr {
			converted, err := convertParamStrToType(input.Name, *input.Type.Elem, p)
			if err != nil {
				return nil, err
			}
			result = append(result, converted.(bool))
		}
		return result, nil
	case abi.AddressTy:
		result := []common.Address{}
		if len(arrayContent) == 0 {
			return result, nil
		}
		for _, p := range paramsStr {
			converted, err := convertParamStrToType(input.Name, *input.Type.Elem, p)
			if err != nil {
				return nil, err
			}
			result = append(result, converted.(common.Address))
		}
		return result, nil
	case abi.HashTy:
		result := []common.Hash{}
		if len(arrayContent) == 0 {
			return result, nil
		}
		for _, p := range paramsStr {
			converted, err := convertParamStrToType(input.Name, *input.Type.Elem, p)
			if err != nil {
				return nil, err
			}
			result = append(result, converted.(common.Hash))
		}
		return result, nil
	case abi.BytesTy:
		return nil, fmt.Errorf("not supported array of type: %s", input.Type.Elem)
	case abi.FixedBytesTy:
		return nil, fmt.Errorf("not supported array of type: %s", input.Type.Elem)
	case abi.FunctionTy:
		return nil, fmt.Errorf("not supported array of type: %s", input.Type.Elem)
	default:
		return nil, fmt.Errorf("not supported array of type: %s", input.Type.Elem)
	}
}

func promptNonArray(input abi.Argument, prefill string) (interface{}, error) {
	var inpStr string
	if prefill == "" {
		inpStr = promptInput("")
	} else {
		inpStr = prefill
	}
	inpStr = strings.Trim(inpStr, " ")
	return convertParamStrToType(input.Name, input.Type, inpStr)
}

func indent(nospace int, str string) string {
	indentation := ""
	for i := 0; i < nospace; i++ {
		indentation += " "
	}
	return strings.ReplaceAll(str, "\n", fmt.Sprintf("\n%s", indentation))
}
