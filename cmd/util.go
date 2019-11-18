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
		indexInput := text[0 : len(text)-1]
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
	return text[0 : len(text)-1]
}

func promptFilePath(prompter string) string {
	return promptInput(prompter)
}

func promptParam(input abi.Argument) (interface{}, error) {
	t := input.Type
	switch t.T {
	case abi.SliceTy, abi.ArrayTy:
		return promptArray(input)
	default:
		return promptNonArray(input)
	}
}

func convertToBytes(str string) ([]byte, error) {
	return nil, fmt.Errorf("not supported")
}

func convertToString(str string) ([]byte, error) {
	str = strings.Trim(str, " ")
	if len(str) < 2 || str[0] != '"' || str[len(str)-1] != '"' {
		return nil, fmt.Errorf(`string must be wrapped by ""`)
	}
	return []byte(str), nil
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
		token, err := convertToAddress(fmt.Sprintf("%s token", tokenName))
		if err != nil {
			return nil, err
		}
		decimal, err := util.GetERC20Decimal(token.Hex(), Network)
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
		return convertToBytes(str)
	case abi.FunctionTy:
		return convertToBytes(str)
	default:
		return nil, fmt.Errorf("not supported type: %s", t)
	}
}

func promptArray(input abi.Argument) (interface{}, error) {
	inpStr := promptInput("")
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
		return nil, fmt.Errorf("not supported type: %s", input.Type.Elem)
	case abi.FixedBytesTy:
		return nil, fmt.Errorf("not supported type: %s", input.Type.Elem)
	case abi.FunctionTy:
		return nil, fmt.Errorf("not supported type: %s", input.Type.Elem)
	default:
		return nil, fmt.Errorf("not supported type: %s", input.Type.Elem)
	}
}

func promptNonArray(input abi.Argument) (interface{}, error) {
	inpStr := promptInput("")
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
