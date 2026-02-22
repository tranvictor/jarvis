package util

import (
	"fmt"
	"math/big"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
)

const (
	NEXT                     int    = -1
	BACK                     int    = -2
	CUSTOM                   int    = -3
	CONSTRUCTOR_METHOD_INDEX uint64 = 1000000 // assuming there is no contract with more than 1m methods
)

type (
	NumberValidator func(number *big.Int) error
	StringValidator func(st string) error
)

// PromptInputWithValidation shows a label, then loops until the validator passes.
func PromptInputWithValidation(u ui.UI, label string, validator StringValidator) string {
	if label != "" {
		u.Info(label)
	}
	return u.Ask(func(s string) error {
		return validator(s)
	})
}

// PromptPercentageBps prompts for a basis-points value in [0, upbound].
func PromptPercentageBps(u ui.UI, label string, upbound int64, network jarvisnetworks.Network) *big.Int {
	return PromptNumber(u, label, func(number *big.Int) error {
		n := number.Int64()
		if n < 0 || n > upbound {
			return fmt.Errorf("this percentage bps must be in [0, %d]", upbound)
		}
		return nil
	}, network)
}

// PromptNumber shows a label and loops until a valid number satisfying validator is entered.
func PromptNumber(u ui.UI, label string, validator NumberValidator, network jarvisnetworks.Network) *big.Int {
	if label != "" {
		u.Info(label)
	}
	var result *big.Int
	u.Ask(func(s string) error {
		num, err := util.ConvertToBig(strings.TrimSpace(s), network)
		if err != nil {
			return fmt.Errorf("couldn't interpret as a number: %s", err)
		}
		if err := validator(num); err != nil {
			return err
		}
		result = num
		return nil
	})
	return result
}

// PromptItemInList shows a label and loops until the user enters one of options.
func PromptItemInList(u ui.UI, label string, options []string) string {
	if label != "" {
		u.Info(label)
	}
	return u.Ask(func(s string) error {
		s = strings.TrimSpace(s)
		for _, op := range options {
			if s == strings.TrimSpace(op) {
				return nil
			}
		}
		return fmt.Errorf("your input is not in the list")
	})
}

// PromptIndex shows a label and loops until the user enters a valid index in
// [min, max] or one of the navigation keywords "next", "back", "custom".
func PromptIndex(u ui.UI, label string, min, max int) int {
	if label != "" {
		u.Info(label)
	}
	for {
		input := strings.TrimSpace(u.Ask(nil))
		switch input {
		case "next":
			return NEXT
		case "back":
			return BACK
		case "custom":
			return CUSTOM
		}
		index, err := strconv.Atoi(input)
		if err != nil {
			u.Error("please enter a number between %d and %d, or 'next' / 'back' / 'custom'", min, max)
			continue
		}
		if min <= index && index <= max {
			return index
		}
		u.Error("please enter a number between %d and %d", min, max)
	}
}

// PromptInput shows an optional label and reads one line.
func PromptInput(u ui.UI, label string) string {
	if label != "" {
		u.Info(label)
	}
	return u.Ask(nil)
}

// PromptFilePath shows an optional label and reads a file path.
func PromptFilePath(u ui.UI, label string) string {
	return PromptInput(u, label)
}

// PromptParam prompts for a single ABI parameter value.
// If prefill is non-empty the user is not prompted; the prefill is used directly.
func PromptParam(
	u ui.UI,
	interactiveMode bool,
	input abi.Argument,
	prefill string,
	network jarvisnetworks.Network,
) (any, error) {
	t := input.Type
	switch t.T {
	case abi.SliceTy, abi.ArrayTy:
		return PromptArray(u, input, prefill, network)
	default:
		return PromptNonArray(u, input, prefill, network)
	}
}

// PromptArray prompts for an array-typed ABI parameter.
func PromptArray(u ui.UI, input abi.Argument, prefill string, network jarvisnetworks.Network) (interface{}, error) {
	var inpStr string
	if prefill == "" {
		inpStr = u.Ask(nil)
	} else {
		inpStr = prefill
	}
	inpStr = strings.TrimSpace(inpStr)
	inpStr, err := util.InterpretInput(inpStr, network)
	if err != nil {
		return nil, err
	}

	paramsStr, err := util.SplitArrayOrTupleStringInput(inpStr)
	if err != nil {
		return nil, err
	}

	switch input.Type.Elem.T {
	case abi.StringTy:
		result := []string{}
		for _, p := range paramsStr {
			converted, err := util.ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
			if err != nil {
				return nil, err
			}
			result = append(result, converted.(string))
		}
		return result, nil
	case abi.IntTy, abi.UintTy:
		switch input.Type.Elem.Size {
		case 8:
			result := []uint8{}
			for _, p := range paramsStr {
				converted, err := util.ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
				if err != nil {
					return nil, err
				}
				result = append(result, converted.(uint8))
			}
			return result, nil
		case 16:
			result := []uint16{}
			for _, p := range paramsStr {
				converted, err := util.ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
				if err != nil {
					return nil, err
				}
				result = append(result, converted.(uint16))
			}
			return result, nil
		case 32:
			result := []uint32{}
			for _, p := range paramsStr {
				converted, err := util.ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
				if err != nil {
					return nil, err
				}
				result = append(result, converted.(uint32))
			}
			return result, nil
		case 64:
			result := []uint64{}
			for _, p := range paramsStr {
				converted, err := util.ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
				if err != nil {
					return nil, err
				}
				result = append(result, converted.(uint64))
			}
			return result, nil
		default:
			result := []*big.Int{}
			for _, p := range paramsStr {
				converted, err := util.ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
				if err != nil {
					return nil, err
				}
				result = append(result, converted.(*big.Int))
			}
			return result, nil
		}
	case abi.BoolTy:
		result := []bool{}
		for _, p := range paramsStr {
			converted, err := util.ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
			if err != nil {
				return nil, err
			}
			result = append(result, converted.(bool))
		}
		return result, nil
	case abi.AddressTy:
		result := []common.Address{}
		for _, p := range paramsStr {
			converted, err := util.ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
			if err != nil {
				return nil, err
			}
			result = append(result, converted.(common.Address))
		}
		return result, nil
	case abi.HashTy:
		result := []common.Hash{}
		for _, p := range paramsStr {
			converted, err := util.ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
			if err != nil {
				return nil, err
			}
			result = append(result, converted.(common.Hash))
		}
		return result, nil
	case abi.BytesTy:
		return nil, fmt.Errorf(
			"not supported array of type: %s - %x",
			input.Type.Elem,
			input.Type.Elem.T,
		)
	case abi.FixedBytesTy:
		return util.ConvertParamStrToFixedByteType(input.Name, *input.Type.Elem, paramsStr, network)
	case abi.FunctionTy:
		return nil, fmt.Errorf(
			"not supported array of type: %s - %x",
			input.Type.Elem,
			input.Type.Elem.T,
		)
	case abi.TupleTy:
		sliceType := reflect.SliceOf(input.Type.Elem.TupleType)
		result := reflect.MakeSlice(sliceType, 0, 0)
		for _, p := range paramsStr {
			converted, err := util.ConvertParamStrToType(input.Name, *input.Type.Elem, p, network)
			if err != nil {
				return nil, err
			}
			result = reflect.Append(result, reflect.ValueOf(converted))
		}
		return result.Interface(), nil
	default:
		return nil, fmt.Errorf(
			"not supported array of type: %s - %x",
			input.Type.Elem,
			input.Type.Elem.T,
		)
	}
}

// PromptNonArray prompts for a scalar ABI parameter value.
func PromptNonArray(u ui.UI, input abi.Argument, prefill string, network jarvisnetworks.Network) (interface{}, error) {
	var inpStr string
	if prefill == "" {
		inpStr = u.Ask(nil)
	} else {
		inpStr = prefill
	}
	inpStr = strings.TrimSpace(inpStr)
	inpStr, err := util.InterpretInput(inpStr, network)
	if err != nil {
		return nil, fmt.Errorf("couldn't interpret input: %w", err)
	}
	return util.ConvertParamStrToType(input.Name, input.Type, inpStr, network)
}

// PromptTxConfirmation displays a transaction summary and asks the user to
// confirm before signing. Returns an error if the user aborts.
func PromptTxConfirmation(
	u ui.UI,
	analyzer util.TxAnalyzer,
	from jarviscommon.Address,
	tx *types.Transaction,
	customABIs map[string]*abi.ABI,
	network jarvisnetworks.Network,
) error {
	u.Section("Confirm tx data before signing")
	if err := showTxInfoToConfirm(u, analyzer, from, tx, customABIs, network); err != nil {
		u.Error("%s", err)
		return err
	}
	if !config.YesToAllPrompt && !u.Confirm("Confirm?", true) {
		return fmt.Errorf("user aborted")
	}
	return nil
}

// PromptTxData guides the user through selecting a method and filling its
// parameters, then returns the ABI-encoded call data.
func PromptTxData(
	u ui.UI,
	analyzer util.TxAnalyzer,
	contractAddress string,
	methodIndex uint64,
	prefills []string,
	prefillMode bool,
	a *abi.ABI,
	customABIs map[string]*abi.ABI,
	network jarvisnetworks.Network,
) ([]byte, error) {
	method, params, err := PromptFunctionCallData(
		u,
		analyzer,
		contractAddress,
		methodIndex,
		prefills,
		prefillMode,
		"write",
		a,
		customABIs,
		network,
	)
	if err != nil {
		return []byte{}, err
	}

	for _, param := range params {
		jarviscommon.DebugPrintf("param: %+v\n", param)
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

// AllZeroParamFunctions returns all read-only ABI methods that take no inputs.
func AllZeroParamFunctions(a *abi.ABI) []abi.Method {
	methods := []abi.Method{}
	for _, m := range a.Methods {
		if m.IsConstant() && len(m.Inputs) == 0 {
			methods = append(methods, m)
		}
	}
	sort.Sort(orderedMethods(methods))
	return methods
}

// PromptMethod lists the available methods and lets the user choose one.
// If methodIndex is non-zero, that method is selected without prompting.
func PromptMethod(u ui.UI, a *abi.ABI, methodIndex uint64, mode string) (*abi.Method, string, error) {
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
		u.Info("%s functions:", mode)
		for i, m := range methods {
			u.Info("%d. %s", i+1, m.Name)
		}
		methodIndex = uint64(
			PromptIndex(
				u,
				fmt.Sprintf("Please choose method index [%d, %d]", 1, len(methods)),
				1,
				len(methods),
			),
		)
		method := &methods[methodIndex-1]
		return method, method.Name, nil
	} else if methodIndex == CONSTRUCTOR_METHOD_INDEX {
		method := a.Constructor
		return &method, "constructor", nil
	} else if int(methodIndex) > len(methods) {
		return nil, "", fmt.Errorf("the contract doesn't have %d(th) %s method", methodIndex, mode)
	} else {
		method := &methods[methodIndex-1]
		return method, method.Name, nil
	}
}

// PromptFunctionCallData guides the user through picking a method and filling
// all its parameters interactively or from prefills.
func PromptFunctionCallData(
	u ui.UI,
	analyzer util.TxAnalyzer,
	contractAddress string,
	methodIndex uint64,
	prefills []string,
	prefillMode bool,
	mode string,
	a *abi.ABI,
	customABIs map[string]*abi.ABI,
	network jarvisnetworks.Network,
) (method *abi.Method, params []any, err error) {
	method, methodName, err := PromptMethod(u, a, methodIndex, mode)
	if err != nil {
		return nil, nil, err
	}

	if method.Type == abi.Constructor {
		u.Info("Creating new contract at %s", contractAddress)
	} else {
		u.Info("Contract: %s", jarviscommon.VerboseAddress(util.GetJarvisAddress(contractAddress, network)))
	}
	u.Info("Method: %s", methodName)

	inputs := method.Inputs
	if prefillMode && len(inputs) != len(prefills) {
		return nil, nil, fmt.Errorf("you must specify enough params in prefilled mode")
	}

	u.Info("Input:")
	paramUI := u.Indent()
	params = []any{}
	pi := 0
	for {
		if pi >= len(inputs) {
			break
		}
		input := inputs[pi]

		paramUI.Info("%d. %s (%s)", pi+1, input.Name, input.Type.String())

		var inputParam any
		if !prefillMode || prefills[pi] == "?" {
			inputParam, err = PromptParam(paramUI, true, input, "", network)
			if err != nil {
				paramUI.Error("your input is not valid: %s", err)
				continue
			}
		} else {
			inputParam, err = PromptParam(paramUI, false, input, prefills[pi], network)
			if err != nil {
				paramUI.Error("your input is not valid: %s", err)
				return nil, nil, fmt.Errorf("your input is not valid: %w", err)
			}
		}

		paramUI.Info("You entered:")
		jarviscommon.PrintVerboseParamResultToWriter(
			paramUI.Indent().Writer(),
			analyzer.ParamAsJarvisParamResult(input.Name, input.Type, inputParam),
			0,
			true,
		)
		fmt.Fprintln(paramUI.Writer())

		params = append(params, inputParam)
		pi++
	}
	return method, params, nil
}

// showTxInfoToConfirm writes the transaction summary (from, to, value, gas,
// decoded function call) to the UI for the user to review before signing.
func showTxInfoToConfirm(
	u ui.UI,
	analyzer util.TxAnalyzer,
	from jarviscommon.Address,
	tx *types.Transaction,
	customABIs map[string]*abi.ABI,
	network jarvisnetworks.Network,
) error {
	if tx.To() != nil {
		u.Critical(
			"from: %s ==> %s",
			jarviscommon.VerboseAddress(from),
			jarviscommon.VerboseAddress(util.GetJarvisAddress(tx.To().Hex(), network)),
		)
	} else {
		cAddr := crypto.CreateAddress(
			jarviscommon.HexToAddress(from.Address),
			tx.Nonce(),
		).Hex()
		u.Critical(
			"from: %s ==> create contract at %s",
			jarviscommon.VerboseAddress(from),
			cAddr,
		)
	}

	sendingETH := jarviscommon.BigToFloatString(tx.Value(), network.GetNativeTokenDecimal())
	if tx.Value().Cmp(big.NewInt(0)) > 0 {
		u.Critical("Value: %s %s", sendingETH, network.GetNativeTokenSymbol())
	}

	switch tx.Type() {
	case types.LegacyTxType:
		u.Critical(
			"Nonce: %d  |  Gas Price: %.4f gwei (%d gas = %.8f %s)",
			tx.Nonce(),
			jarviscommon.BigToFloat(tx.GasPrice(), 9),
			tx.Gas(),
			jarviscommon.BigToFloat(
				big.NewInt(0).Mul(big.NewInt(int64(tx.Gas())), tx.GasPrice()),
				18,
			),
			network.GetNativeTokenSymbol(),
		)
	case types.DynamicFeeTxType:
		u.Critical(
			"Nonce: %d  |  Max Gas Price: %.4f gwei, Max Tip Price: %.4f gwei (%d gas = %.8f %s)",
			tx.Nonce(),
			jarviscommon.BigToFloat(tx.GasFeeCap(), 9),
			jarviscommon.BigToFloat(tx.GasTipCap(), 9),
			tx.Gas(),
			jarviscommon.BigToFloat(
				big.NewInt(0).Mul(big.NewInt(int64(tx.Gas())), tx.GasPrice()),
				18,
			),
			network.GetNativeTokenSymbol(),
		)
	}

	if tx.To() == nil {
		return nil
	}

	isContract, err := util.IsContract(tx.To().Hex(), network)
	if err != nil {
		return err
	}
	if !isContract {
		return nil
	}

	fc := analyzer.AnalyzeFunctionCallRecursively(
		util.GetABI,
		tx.Value(),
		tx.To().Hex(),
		tx.Data(),
		customABIs,
	)
	jarviscommon.PrintFunctionCallToWriter(fc, u.Writer())
	fmt.Fprintln(os.Stdout) // blank line after the function call block
	return nil
}
