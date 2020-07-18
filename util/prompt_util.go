package util

import (
	"bufio"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const (
	NEXT int = -1
	BACK int = -2
)

type NumberValidator func(number *big.Int) error
type StringValidator func(st string) error

func PromptInputWithValidation(prompter string, validator StringValidator) string {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s: ", prompter)
		text, _ := reader.ReadString('\n')
		result := strings.Trim(text[0:len(text)-1], "\r\n")
		err := validator(result)
		if err == nil {
			return result
		}
		fmt.Printf("Jarvis: %s\n", err)
	}
}

func PromptPercentageBps(prompter string, upbound int64, network string) *big.Int {
	return PromptNumber(prompter, func(number *big.Int) error {
		n := number.Int64()
		if n < 0 || n > upbound {
			return fmt.Errorf("This percentage bps must be in [0, %d]", upbound)
		}
		return nil
	}, network)
}

func PromptNumber(prompter string, validator NumberValidator, network string) *big.Int {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s: ", prompter)
		text, _ := reader.ReadString('\n')
		input := strings.Trim(text[0:len(text)-1], "\r\n ")
		num, err := ConvertToBig(input, network)
		if err != nil {
			fmt.Printf("Jarvis: couldn't interpret as a number because %s\n", err)
			continue
		}
		err = validator(num)
		if err == nil {
			return num
		}
		fmt.Printf("Jarvis: %s\n", err)
	}
}

func PromptItemInList(prompter string, options []string) string {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s: ", prompter)
		text, _ := reader.ReadString('\n')
		input := strings.Trim(text[0:len(text)-1], "\r\n ")
		for _, op := range options {
			if input == strings.Trim(op, "\r\n ") {
				return input
			}
		}
		fmt.Printf("Jarvis: Your input is not in the list.\n")
	}
}

func PromptIndex(prompter string, min, max int) int {
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

func PromptInput(prompter string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s: ", prompter)
	text, _ := reader.ReadString('\n')
	return strings.Trim(text[0:len(text)-1], "\r\n")
}

func PromptFilePath(prompter string) string {
	return PromptInput(prompter)
}

func PromptParam(input abi.Argument, prefill string, network string) (interface{}, error) {
	t := input.Type
	switch t.T {
	case abi.SliceTy, abi.ArrayTy:
		return PromptArray(input, prefill, network)
	default:
		return PromptNonArray(input, prefill, network)
	}
}

func PromptArray(input abi.Argument, prefill string, network string) (interface{}, error) {
	var inpStr string
	if prefill == "" {
		inpStr = PromptInput("")
	} else {
		inpStr = prefill
	}
	inpStr = strings.Trim(inpStr, " ")
	if len(inpStr) < 2 || inpStr[0] != '[' || inpStr[len(inpStr)-1] != ']' {
		return nil, fmt.Errorf("input must be wrapped by []")
	}
	arrayContent := strings.Trim(inpStr[1:len(inpStr)-1], " ")
	paramsStr := []string{}
	for _, p := range strings.Split(arrayContent, ",") {
		if strings.Trim(p, " ") != "" {
			paramsStr = append(paramsStr, p)
		}
	}

	switch input.Type.Elem.T {
	case abi.StringTy: // variable arrays are written at the end of the return bytes
		result := []string{}
		if len(arrayContent) == 0 {
			return result, nil
		}
		for _, p := range paramsStr {
			converted, err := ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
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
			converted, err := ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
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
			converted, err := ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
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
			converted, err := ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
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
			converted, err := ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
			if err != nil {
				return nil, err
			}
			result = append(result, converted.(common.Hash))
		}
		return result, nil
	case abi.BytesTy:
		return nil, fmt.Errorf("not supported array of type: %s", input.Type.Elem)
	case abi.FixedBytesTy:
		return ConvertParamStrToFixedByteType(input.Name, *input.Type.Elem, paramsStr, network)
	case abi.FunctionTy:
		return nil, fmt.Errorf("not supported array of type: %s", input.Type.Elem)
	default:
		return nil, fmt.Errorf("not supported array of type: %s", input.Type.Elem)
	}
}

func PromptNonArray(input abi.Argument, prefill string, network string) (interface{}, error) {
	var inpStr string
	if prefill == "" {
		inpStr = PromptInput("")
	} else {
		inpStr = prefill
	}
	inpStr = strings.Trim(inpStr, " ")
	return ConvertParamStrToType(input.Name, input.Type, inpStr, network)
}
