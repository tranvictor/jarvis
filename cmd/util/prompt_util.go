package util

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	jarviscommon "github.com/tranvictor/jarvis/common"
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
func echoParam(u ui.UI, analyzer util.TxAnalyzer, contract string, input abi.Argument, value any, raw string, fold bool) {
	// Only integers can be token amounts; skip the ERC-20 lookup otherwise.
	if t := input.Type.T; t != abi.UintTy && t != abi.IntTy {
		contract = ""
	}
	d := util.NewParamDisplay(analyzer.ParamAsJarvisParamResultFor(contract, input.Name, input.Type, value))
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

// nextSigningNote carries context from a caller that knows what the next EOA
// transaction is for (e.g. runSafeExecute) into PromptTxConfirmation, which
// only sees the raw transaction. It is consumed by the next confirmation.
var nextSigningNote *SigningNote

// SigningNote annotates the next EOA signing card.
type SigningNote struct {
	// ExecutesSafeTxHash marks the tx as the execTransaction of a Safe tx the
	// user has just reviewed, so the card links to it and collapses the call.
	ExecutesSafeTxHash string
	// WalletName and WalletKind describe the signing account; the name fills
	// in for an address-book entry when there is none, the kind is shown
	// next to the signer.
	WalletName string
	WalletKind string
	// SignerBalance, when known, enables the insufficient-funds warning.
	SignerBalance *big.Int
}

// SetNextSigningNote attaches a note to the next PromptTxConfirmation call.
func SetNextSigningNote(n SigningNote) { nextSigningNote = &n }

// mergeSigningNote fills the empty fields of the pending note from n, so a
// caller that only knows the wallet does not erase a caller that only knows
// the Safe context (or vice versa).
func mergeSigningNote(n SigningNote) {
	if nextSigningNote == nil {
		nextSigningNote = &SigningNote{}
	}
	if nextSigningNote.ExecutesSafeTxHash == "" {
		nextSigningNote.ExecutesSafeTxHash = n.ExecutesSafeTxHash
	}
	if nextSigningNote.WalletName == "" {
		nextSigningNote.WalletName = n.WalletName
	}
	if nextSigningNote.WalletKind == "" {
		nextSigningNote.WalletKind = n.WalletKind
	}
	if nextSigningNote.SignerBalance == nil {
		nextSigningNote.SignerBalance = n.SignerBalance
	}
}

// ErrUserCancelled is returned when the operator declines the signing card.
// Nothing has been signed or sent at that point; callers should exit quietly.
var ErrUserCancelled = errors.New("cancelled by user")

// PromptTxConfirmation displays the signing card for tx and asks the user to
// confirm before signing. Returns an error if the user aborts.
func PromptTxConfirmation(
	u ui.UI,
	analyzer util.TxAnalyzer,
	from jarviscommon.Address,
	tx *types.Transaction,
	customABIs map[string]*abi.ABI,
	network jarvisnetworks.Network,
) error {
	note := nextSigningNote
	nextSigningNote = nil
	card, err := buildEOASigningCard(u, analyzer, from, tx, customABIs, network, note)
	if err != nil {
		u.Error("%s", err)
		return err
	}
	if !ConfirmSigningCard(u, card) {
		u.Warn("Cancelled — nothing was signed or sent.")
		return ErrUserCancelled
	}
	return nil
}

// buildEOASigningCard assembles the SigningCard for a raw EOA transaction:
// resolves the destination, decodes the calldata when it targets a contract,
// and derives the warnings.
func buildEOASigningCard(
	u ui.UI,
	analyzer util.TxAnalyzer,
	from jarviscommon.Address,
	tx *types.Transaction,
	customABIs map[string]*abi.ABI,
	network jarvisnetworks.Network,
	note *SigningNote,
) (*SigningCard, error) {
	symbol := network.GetNativeTokenSymbol()
	// The card shows checksummed hex whatever form the wallet file stored.
	from.Address = jarviscommon.HexToAddress(from.Address).Hex()
	if note != nil && note.WalletName != "" && !jarviscommon.IsKnownAddress(from) {
		from.Desc = note.WalletName
	}
	legacy := tx.Type() == types.LegacyTxType
	price := tx.GasFeeCap()
	if legacy {
		price = tx.GasPrice()
	}
	card := &SigningCard{
		Kind:    "EOA transaction",
		Network: network.GetName(),
		Signer:  util.StyledAddress(from),
		Nonce:   fmt.Sprintf("%d", tx.Nonce()),
		Gas:     FormatGasLine(legacy, tx.GasPrice(), tx.GasFeeCap(), tx.GasTipCap(), tx.Gas(), symbol),
	}
	if note != nil {
		card.Wallet = note.WalletKind
	}
	if tx.Value().Sign() > 0 {
		card.Value = jarviscommon.BigToFloatString(tx.Value(), network.GetNativeTokenDecimal()) + " " + symbol
	}
	maxCost := new(big.Int).Add(tx.Value(), MaxGasCost(price, tx.Gas()))
	var balance *big.Int
	if note != nil {
		balance = note.SignerBalance
	}

	if tx.To() == nil {
		card.CreateAddress = crypto.CreateAddress(jarviscommon.HexToAddress(from.Address), tx.Nonce()).Hex()
		if len(tx.Data()) > 0 {
			card.RawData = "0x" + ethcommon.Bytes2Hex(tx.Data())
		}
		card.Warnings = SigningWarnings(WarningInput{
			Value: tx.Value(), NativeSymbol: symbol, SignerBalance: balance, MaxCost: maxCost,
		})
		card.Prompt = fmt.Sprintf("Sign and broadcast contract creation (%s)?", gasCostOnly(card.Gas))
		return card, nil
	}

	toHex := tx.To().Hex()
	if isERC20, _ := util.IsERC20(toHex, network); isERC20 {
		// Warm the caches so the analyzer below annotates token amounts.
		util.GetERC20Symbol(toHex, network)
		util.GetERC20Decimal(toHex, network)
	}
	to := util.GetJarvisAddress(toHex, network)
	card.To = util.StyledAddress(to)

	isContract, err := util.IsContract(toHex, network)
	if err != nil {
		return nil, err
	}
	warn := WarningInput{
		To: to, ToIsContract: isContract, Value: tx.Value(), NativeSymbol: symbol,
		HasData: len(tx.Data()) > 0, SignerBalance: balance, MaxCost: maxCost,
	}

	var fc *jarviscommon.FunctionCall
	if isContract && len(tx.Data()) > 0 {
		fc = analyzer.AnalyzeFunctionCallRecursively(util.GetABI, tx.Value(), toHex, tx.Data(), customABIs)
		warn.Call = fc
		if fc != nil {
			card.Call = util.NewFunctionCallDisplay(fc)
		}
		// ERC-7730 clear-signing layer: when a descriptor matches the
		// destination contract, the curated view is rendered above the raw
		// ABI decode so the operator can scan the intent at a glance and
		// still cross-check against the full breakdown. Failures fall through
		// silently — we never make the review worse than today.
		if fc != nil && fc.Method != "" {
			card.ClearSign = func(cu ui.UI) { renderContractClearSign(cu, tx, fc, network, customABIs) }
		}
	} else if len(tx.Data()) > 0 {
		card.RawData = "0x" + ethcommon.Bytes2Hex(tx.Data())
	}

	if note != nil && note.ExecutesSafeTxHash != "" {
		card.Kind = "Safe execution"
		card.CollapseCall = card.Call != nil && card.Call.Method == "execTransaction"
		card.Safe = &SafeCardFields{Executes: note.ExecutesSafeTxHash}
		// Safe params were already shown on the Safe card; only the EOA facts
		// remain on this one.
		card.Safe.Operation, card.Safe.SafeNonce, card.Safe.SafeTxHash = "", "", ""
	}

	card.Warnings = SigningWarnings(warn)
	card.Prompt = fmt.Sprintf("Sign and broadcast (%s)?", gasCostOnly(card.Gas))
	return card, nil
}

// gasCostOnly extracts the "≈ 0.0017 ETH" head of a FormatGasLine string.
func gasCostOnly(gas string) string {
	if i := strings.Index(gas, "   ("); i >= 0 {
		return gas[:i]
	}
	return gas
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
			if hint := inputHint(input.Type, network); hint != "" && interactive {
				paramUI.Info("%s", paramUI.Style(ui.StyledText{Text: hint, Severity: ui.SeverityMuted}))
			}
			if !interactive {
				return nil, nil, fmt.Errorf("your input is not valid: %w", err)
			}
			continue
		}

		echoParam(paramUI, analyzer, contractAddress, input, inputParam, raw, interactive)
		params = append(params, inputParam)
		pi++
	}
	return method, params, nil
}

// inputHint is the one-line reminder of the accepted input forms for a type,
// shown after a rejected answer so the operator doesn't have to guess.
func inputHint(t abi.Type, network jarvisnetworks.Network) string {
	switch t.T {
	case abi.UintTy, abi.IntTy:
		return fmt.Sprintf("accepted: raw integer (1000000), hex (0xf4240), or amount with token (0.5 %s, 1000 USDC)",
			network.GetNativeTokenSymbol())
	case abi.AddressTy:
		return "accepted: 0x address or an address-book name (jarvis addr to list)"
	case abi.BoolTy:
		return "accepted: true / false"
	case abi.BytesTy, abi.FixedBytesTy:
		return "accepted: 0x-prefixed hex"
	case abi.SliceTy, abi.ArrayTy:
		return "accepted: comma-separated items in brackets, e.g. [a, b, c]"
	}
	return ""
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
