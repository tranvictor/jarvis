package util

import (
	"bufio"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/Songmu/prompter"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tranvictor/ethutils"
	. "github.com/tranvictor/jarvis/common"
	. "github.com/tranvictor/jarvis/networks"
)

const (
	NEXT                     int    = -1
	BACK                     int    = -2
	CONSTRUCTOR_METHOD_INDEX uint64 = 1000000 // assuming there is no contract with more than 1m methods
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

func PromptPercentageBps(prompter string, upbound int64, network Network) *big.Int {
	return PromptNumber(prompter, func(number *big.Int) error {
		n := number.Int64()
		if n < 0 || n > upbound {
			return fmt.Errorf("This percentage bps must be in [0, %d]", upbound)
		}
		return nil
	}, network)
}

func PromptNumber(prompter string, validator NumberValidator, network Network) *big.Int {
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

func PromptParam(input abi.Argument, prefill string, network Network) (interface{}, error) {
	t := input.Type
	switch t.T {
	case abi.SliceTy, abi.ArrayTy:
		return PromptArray(input, prefill, network)
	default:
		return PromptNonArray(input, prefill, network)
	}
}

func PromptArray(input abi.Argument, prefill string, network Network) (interface{}, error) {
	var inpStr string
	if prefill == "" {
		inpStr = PromptInput("")
	} else {
		inpStr = prefill
	}
	inpStr = strings.Trim(inpStr, " ")
	inpStr, err := InterpretInput(inpStr, network)
	if err != nil {
		return nil, err
	}
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

func PromptNonArray(input abi.Argument, prefill string, network Network) (interface{}, error) {
	var inpStr string
	if prefill == "" {
		inpStr = PromptInput("")
	} else {
		inpStr = prefill
	}
	inpStr = strings.Trim(inpStr, " ")
	inpStr, err := InterpretInput(inpStr, network)
	if err != nil {
		return nil, err
	}
	return ConvertParamStrToType(input.Name, input.Type, inpStr, network)
}

func PromptTxConfirmation(
	analyzer TxAnalyzer,
	from Address,
	tx *types.Transaction,
	customABIs map[string]*abi.ABI,
	network Network,
) error {
	fmt.Printf("\n========== Confirm tx data before signing ==========\n\n")
	err := showTxInfoToConfirm(
		analyzer, from, tx, customABIs, network,
	)
	if err != nil {
		fmt.Printf("%s\n", err)
		return err
	}
	if !prompter.YN("\nConfirm?", true) {
		return fmt.Errorf("user aborted")
	}
	return nil
}

func indent(nospace int, strs []string) string {
	if len(strs) == 0 {
		return ""
	}

	if len(strs) == 1 {
		return strs[0]
	}

	indentation := ""
	for i := 0; i < nospace; i++ {
		indentation += " "
	}
	result := ""
	for i, str := range strs {
		result += fmt.Sprintf("\n%s%d. %s", indentation, i, str)
	}
	result += "\n"
	return result
}

func PromptTxData(
	analyzer TxAnalyzer,
	contractAddress string,
	methodIndex uint64,
	prefills []string,
	prefillMode bool,
	a *abi.ABI,
	customABIs map[string]*abi.ABI,
	network Network) ([]byte, error) {
	method, params, err := PromptFunctionCallData(
		analyzer,
		contractAddress,
		methodIndex,
		prefills,
		prefillMode,
		"write",
		a,
		customABIs,
		network)
	if err != nil {
		return []byte{}, err
	}
	if method.Type == abi.Constructor {
		return method.Inputs.Pack(params...)
	}
	return a.Pack(method.Name, params...)
}

type orderedMethods []abi.Method

func (m orderedMethods) Len() int           { return len(m) }
func (m orderedMethods) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m orderedMethods) Less(i, j int) bool { return m[i].Name < m[j].Name }

func PromptMethod(a *abi.ABI, methodIndex uint64, mode string) (*abi.Method, string, error) {
	methods := []abi.Method{}
	if mode == "write" {
		for _, m := range a.Methods {
			if !m.IsConstant() {
				methods = append(methods, m)
			}
		}
	} else {
		for _, m := range a.Methods {
			if m.IsConstant() {
				methods = append(methods, m)
			}
		}
	}
	sort.Sort(orderedMethods(methods))
	if methodIndex == 0 {
		fmt.Printf("write functions:\n")
		for i, m := range methods {
			fmt.Printf("%d. %s\n", i+1, m.Name)
		}
		methodIndex = uint64(PromptIndex(fmt.Sprintf("Please choose method index [%d, %d]", 1, len(methods)), 1, len(methods)))
		method := &methods[methodIndex-1]
		return method, method.Name, nil
	} else if methodIndex == CONSTRUCTOR_METHOD_INDEX {
		method := a.Constructor
		return &method, "constructor", nil
	} else if int(methodIndex) > len(methods) {
		return nil, "", fmt.Errorf("the contract doesn't have %d(th) write method", methodIndex)
	} else {
		method := &methods[methodIndex-1]
		return method, method.Name, nil
	}
}

func PromptFunctionCallData(
	analyzer TxAnalyzer,
	contractAddress string,
	methodIndex uint64,
	prefills []string,
	prefillMode bool,
	mode string,
	a *abi.ABI,
	customABIs map[string]*abi.ABI,
	network Network) (method *abi.Method, params []interface{}, err error) {

	method, methodName, err := PromptMethod(a, methodIndex, mode)
	if err != nil {
		return nil, nil, err
	}

	if method.Type == abi.Constructor {
		fmt.Printf("Creating new contract at %s\n", contractAddress)
	} else {
		fmt.Printf("\nContract: %s\n", VerboseAddress(GetJarvisAddress(contractAddress, network)))
	}
	fmt.Printf("Method: %s\n", methodName)
	inputs := method.Inputs
	if prefillMode && len(inputs) != len(prefills) {
		return nil, nil, fmt.Errorf("You must specify enough params in prefilled mode")
	}
	fmt.Printf("Input:\n")
	params = []interface{}{}
	pi := 0
	for {
		if pi >= len(inputs) {
			break
		}
		input := inputs[pi]
		var inputParam interface{}
		fmt.Printf("%d. %s (%s)", pi+1, input.Name, input.Type.String())
		if !prefillMode || prefills[pi] == "?" {
			inputParam, err = PromptParam(input, "", network)
			if err != nil {
				fmt.Printf("Your input is not valid: %s\n", err)
				continue
			}

			fmt.Printf(
				"    You entered: %s\n",
				indent(8, VerboseValues(analyzer.ParamAsJarvisValues(input.Type, inputParam))),
			)
		} else {
			inputParam, err = PromptParam(input, prefills[pi], network)
			if err != nil {
				fmt.Printf("Your input is not valid: %s\n", err)
				continue
			}

			fmt.Printf(
				": %s\n",
				indent(8, VerboseValues(analyzer.ParamAsJarvisValues(input.Type, inputParam))),
			)
		}
		params = append(params, inputParam)
		pi++
	}
	return method, params, nil
}

func showTxInfoToConfirm(
	analyzer TxAnalyzer,
	from Address,
	tx *types.Transaction,
	customABIs map[string]*abi.ABI,
	network Network,
) error {
	if tx.To() != nil {
		fmt.Printf(
			"From: %s ==> %s\n",
			VerboseAddress(from),
			VerboseAddress(GetJarvisAddress(tx.To().Hex(), network)),
		)
	} else {
		cAddr := crypto.CreateAddress(
			ethutils.HexToAddress(from.Address),
			tx.Nonce(),
		).Hex()
		fmt.Printf(
			"From: %s ==> Create contract at %s\n",
			VerboseAddress(from),
			cAddr,
		)
	}

	sendingETH := BigToFloatString(tx.Value(), network.GetNativeTokenDecimal())
	if tx.Value().Cmp(big.NewInt(0)) > 0 {
		fmt.Printf("Value: %s\n", InfoColor(fmt.Sprintf("%s %s", sendingETH, network.GetNativeTokenSymbol())))
	}

	fmt.Printf(
		"Nonce: %d  |  Gas: %.4f gwei (%d gas = %.8f %s)\n",
		tx.Nonce(),
		ethutils.BigToFloat(tx.GasPrice(), 9),
		tx.Gas(),
		ethutils.BigToFloat(
			big.NewInt(0).Mul(
				big.NewInt(int64(tx.Gas())),
				tx.GasPrice(),
			),
			18,
		),
		network.GetNativeTokenSymbol(),
	)

	if tx.To() == nil {
		// TODO: analyzing creation tx
		// just ignore it for now
		return nil
	}

	isContract, err := IsContract(tx.To().Hex(), network)
	if err != nil {
		return err
	}

	if !isContract {
		return nil
	}

	fc := analyzer.AnalyzeFunctionCallRecursively(
		GetABI,
		tx.Value(),
		tx.To().Hex(),
		tx.Data(),
		customABIs,
	)
	PrintFunctionCall(fc)

	return nil
}
