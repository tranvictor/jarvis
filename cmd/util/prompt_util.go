package util

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/txanalyzer/erc7730"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
)

const (
	NEXT                     int    = -1
	BACK                     int    = -2
	CUSTOM                   int    = -3
	CONSTRUCTOR_METHOD_INDEX uint64 = 1000000 // assuming there is no contract with more than 1m methods
)

type StringValidator func(st string) error

// PromptInputWithValidation shows a label, then loops until the validator passes.
func PromptInputWithValidation(u ui.UI, label string, validator StringValidator) string {
	if label != "" {
		u.Info(label)
	}
	return u.Ask(func(s string) error {
		return validator(s)
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
	raw := prefill
	if raw == "" {
		raw = u.Ask(nil)
	}
	return ConvertParamInput(input, raw, network)
}

// ConvertParamInput turns one line of user input into the typed value the ABI
// encoder expects for input. Arrays and tuples are entered as a single
// bracketed line ("[a, b]", "(a, b)") and delegated to ConvertParamStrToArray,
// which uses abi.Type.GetType() via reflect to build the exact slice type.
func ConvertParamInput(input abi.Argument, raw string, network jarvisnetworks.Network) (any, error) {
	inpStr, err := util.InterpretInput(strings.TrimSpace(raw), network)
	if err != nil {
		return nil, fmt.Errorf("couldn't interpret input: %w", err)
	}
	switch input.Type.T {
	case abi.SliceTy, abi.ArrayTy:
		return util.ConvertParamStrToArray(input.Name, input.Type, inpStr, network)
	default:
		return util.ConvertParamStrToType(input.Name, input.Type, inpStr, network)
	}
}

// foldableInputLen is the longest typed answer we fold into the "→" line on a
// TTY. RewriteLastLine can only clear one physical row, so an answer that may
// have wrapped is left in place rather than risk leaving fragments behind.
const foldableInputLen = 60

// echoParam shows what jarvis understood from the user's answer. The first
// line replaces the raw "> answer" row on a TTY (when it is short enough to be
// sure it occupied one row); nested values follow as an indented tree.
func echoParam(u ui.UI, analyzer util.TxAnalyzer, input abi.Argument, value any, raw string, fold bool) {
	d := util.NewParamDisplay(analyzer.ParamAsJarvisParamResult(input.Name, input.Type, value))
	lines := util.ParamValueLines(u, d)
	if len(lines) == 0 {
		return
	}
	first := "  → " + lines[0]
	if fold && len(raw) <= foldableInputLen {
		u.RewriteLastLine(first)
	} else {
		u.Info("%s", first)
	}
	for _, l := range lines[1:] {
		u.Info("    %s", l)
	}
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
	if len(methods) == 0 {
		return nil, "", fmt.Errorf("the contract has no %s methods", mode)
	}
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

	contract := util.StyledAddress(util.GetJarvisAddress(contractAddress, network))
	if method.Type == abi.Constructor {
		u.Info("%s  →  new contract at %s", u.Style(ui.StyledText{Text: methodName, Severity: ui.SeverityCritical}), contractAddress)
	} else {
		u.Info("%s  →  %s", u.Style(ui.StyledText{Text: methodName, Severity: ui.SeverityCritical}), u.Style(contract))
	}

	inputs := method.Inputs
	if prefillMode && len(inputs) != len(prefills) {
		return nil, nil, fmt.Errorf("you must specify enough params in prefilled mode")
	}

	paramUI := u.Indent()
	params = []any{}
	for pi := 0; pi < len(inputs); {
		input := inputs[pi]
		paramUI.Info("%d. %s  %s", pi+1, input.Name,
			paramUI.Style(ui.StyledText{Text: input.Type.String(), Severity: ui.SeverityMuted}))

		interactive := !prefillMode || prefills[pi] == "?"
		raw := ""
		if interactive {
			raw = paramUI.Ask(nil)
		} else {
			raw = prefills[pi]
		}

		inputParam, err := ConvertParamInput(input, raw, network)
		if err != nil {
			paramUI.Error("✗ %s", err)
			if !interactive {
				return nil, nil, fmt.Errorf("your input is not valid: %w", err)
			}
			continue
		}

		echoParam(paramUI, analyzer, input, inputParam, raw, interactive)
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
	fromStyled := util.StyledAddress(from)
	u.Critical("From  : %s", u.Style(fromStyled))

	if tx.To() != nil {
		toHex := tx.To().Hex()
		if isERC20, _ := util.IsERC20(toHex, network); isERC20 {
			util.GetERC20Symbol(toHex, network)
			util.GetERC20Decimal(toHex, network)
		}
		toStyled := util.StyledAddress(util.GetJarvisAddress(toHex, network))
		u.Critical("To    : %s", u.Style(toStyled))
	} else {
		cAddr := crypto.CreateAddress(
			jarviscommon.HexToAddress(from.Address),
			tx.Nonce(),
		).Hex()
		u.Critical("To    : create contract at %s", cAddr)
	}

	if tx.Value().Sign() > 0 {
		sendingETH := jarviscommon.BigToFloatString(tx.Value(), network.GetNativeTokenDecimal())
		u.Critical("Value : %s %s", sendingETH, network.GetNativeTokenSymbol())
	}

	gasCost := jarviscommon.BigToFloat(
		big.NewInt(0).Mul(big.NewInt(int64(tx.Gas())), tx.GasPrice()),
		18,
	)
	switch tx.Type() {
	case types.LegacyTxType:
		u.Critical("Nonce : %d", tx.Nonce())
		u.Critical("Gas   : %.4f gwei (%d gas = %.8f %s)",
			jarviscommon.BigToFloat(tx.GasPrice(), 9),
			tx.Gas(), gasCost, network.GetNativeTokenSymbol(),
		)
	case types.DynamicFeeTxType:
		u.Critical("Nonce : %d", tx.Nonce())
		u.Critical("Gas   : Max %.4f gwei, Tip %.4f gwei (%d gas = %.8f %s)",
			jarviscommon.BigToFloat(tx.GasFeeCap(), 9),
			jarviscommon.BigToFloat(tx.GasTipCap(), 9),
			tx.Gas(), gasCost, network.GetNativeTokenSymbol(),
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

	// ERC-7730 clear-signing layer: when a descriptor matches the
	// destination contract, render the curated green-bordered view
	// above the raw ABI decode so the operator can scan the intent
	// at a glance and still cross-check against the full breakdown.
	// Failures (descriptor not found, registry miss, formatter
	// error) silently fall through to the existing display — we
	// never make the review worse than today.
	if fc != nil && fc.Method != "" {
		renderContractClearSign(u, tx, fc, network, customABIs)
	}

	util.DisplayFunctionCall(u, fc)
	u.Info("")
	return nil
}

// renderContractClearSign asks the shared ERC-7730 engine for a
// ClearSignedView of this transaction and, when one is available,
// emits it inside a green-bordered box. The engine is lazy-loaded
// process-wide so first invocation pays a one-time directory walk
// (and a registry sync on the very first run with no local cache).
func renderContractClearSign(
	u ui.UI,
	tx *types.Transaction,
	fc *jarviscommon.FunctionCall,
	network jarvisnetworks.Network,
	customABIs map[string]*abi.ABI,
) {
	var contractABI *abi.ABI
	if customABIs != nil {
		contractABI = customABIs[strings.ToLower(tx.To().Hex())]
	}
	if contractABI == nil {
		if a, err := util.GetABI(tx.To().Hex(), network); err == nil {
			contractABI = a
		}
	}
	engine := erc7730.DefaultEngine()
	view, err := engine.ContractView(
		context.Background(),
		network.GetChainID(),
		tx.To().Hex(),
		tx.Value(),
		tx.Data(),
		fc.Params,
		contractABI,
	)
	if err != nil || view == nil {
		return
	}
	erc7730.Render(u, view)
}
