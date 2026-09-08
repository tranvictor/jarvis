package util

import (
	"fmt"
	"math/big"
	"strings"

	ethcommon "github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
)

// SigningCard is everything shown to the user right before they sign. The
// same renderer serves plain EOA transactions and the Safe flows so the
// screen has one shape wherever a signature is requested.
//
// Addresses on this screen are always complete: the card is the last place a
// lookalike address can be caught.
type SigningCard struct {
	Kind    string // "EOA transaction", "Safe proposal", "Safe approval", "Safe execution", "Classic multisig transaction"
	Network string

	Signer ui.StyledText
	// Wallet is the signing device or key kind ("ledger", "trezor",
	// "keystore"); shown muted next to the signer so a hardware prompt is
	// expected rather than a surprise.
	Wallet string
	To     ui.StyledText // empty Text for contract creation; see CreateAddress
	// ToLabel overrides the destination row name ("Send to", "Safe calls",
	// "Calls"). Native ETH sends to an EOA use "Recipient".
	ToLabel string
	// CreateAddress is the predicted address when the tx deploys a contract.
	CreateAddress string
	Value         string // "1.5 ETH"; empty hides the row
	Gas           string // "max 20.0 gwei, tip 1.5 gwei · 85,123 gas · ≈ 0.0017 ETH"; empty hides
	Nonce         string

	Safe    *SafeCardFields
	Classic *ClassicCardFields

	// Call is the decoded calldata. Nil when there is none or it could not be
	// analyzed; RawData is shown instead when set.
	Call    *util.FunctionCallDisplay
	RawData string
	// CollapseCall prints the call as a single header line. Used when the
	// same call was fully displayed moments ago (Safe approve → execute,
	// Classic info → confirm).
	CollapseCall bool
	// CollapseNote is the muted suffix on a collapsed call header. Empty
	// means "(Safe transaction shown above)".
	CollapseNote string

	// ClearSign renders the ERC-7730 view when a descriptor matched. It is
	// a callback so this package does not depend on the erc7730 engine.
	ClearSign func(ui.UI)

	Warnings []string
	Prompt   string
}

// SafeCardFields are the SafeTx parameters that have no EOA equivalent.
type SafeCardFields struct {
	Operation    string // "CALL" or "DELEGATECALL"
	DelegateCall bool
	MultiSend    bool
	// Address is the Safe itself. Shown as the "Safe" row and used as the
	// payer on Send headlines (ERC-20 transfer / native value).
	Address        ui.StyledText
	SafeNonce      string
	SafeTxHash     string
	SafeTxGas      string
	BaseGas        string
	GasPrice       string
	GasToken       string
	RefundReceiver string
	// Signatures lists owners that already signed, one rendered line each;
	// Threshold (when known) is shown next to the count.
	Signatures []ui.StyledText
	Threshold  uint64
	// Executes is the safeTxHash an EOA execTransaction carries out.
	Executes string
}

// ClassicCardFields are the Gnosis Classic tx parameters that have no EOA
// equivalent: the on-chain tx id, confirmation progress, and confirmer list.
type ClassicCardFields struct {
	TxID          string
	Multisig      ui.StyledText
	Executed      bool
	Confirmations int
	Threshold     uint64
	Signatures    []ui.StyledText
}

// isMultisigOp is true for the inner Classic/Safe transaction the operator
// is reviewing. Those cards are boxed so they stand apart from the EOA
// confirm/execute wrapper that often follows.
func isMultisigOp(c *SigningCard) bool {
	if c == nil {
		return false
	}
	if c.Classic != nil {
		return true
	}
	switch c.Kind {
	case "Classic multisig transaction", "Safe approval", "Safe proposal", "Safe transaction":
		return true
	default:
		return false
	}
}

func signingCardPayer(c *SigningCard) ui.StyledText {
	if c == nil {
		return ui.StyledText{}
	}
	if c.Classic != nil && c.Classic.Multisig.Text != "" {
		return c.Classic.Multisig
	}
	if c.Safe != nil && c.Safe.Address.Text != "" {
		return c.Safe.Address
	}
	return ui.StyledText{}
}

// ShowSigningCard prints the card. Order is chosen so that what the reader
// must verify sits directly above the prompt: the call first, then the
// summary block, then the warnings.
//
// Multisig operations (Classic inner tx, Safe approval/proposal) render
// inside a rounded box so they are the thing the eye hits when a new
// batch item appears. The following EOA confirm/execute card stays a
// plain heading — same [i/n] stamp, no second equals-rule or box.
func ShowSigningCard(u ui.UI, c *SigningCard) {
	title := AnnotateBatch(c.Kind)
	switch {
	case isMultisigOp(c):
		u.BoxedSection(ui.SeverityCritical, title, func(inner ui.UI) {
			renderSigningCardBody(inner, c)
		})
	case c.CollapseCall:
		u.Subsection(title)
		renderSigningCardBody(u, c)
	default:
		u.Section(title)
		renderSigningCardBody(u, c)
	}
}

func renderSigningCardBody(u ui.UI, c *SigningCard) {
	if c.ClearSign != nil {
		c.ClearSign(u)
	}

	if from := signingCardPayer(c); from.Text != "" {
		util.AnnotatePaymentFrom(c.Call, from)
	}

	body := c.ClearSign != nil || c.Call != nil || c.RawData != ""
	switch {
	case c.Call != nil && c.CollapseCall:
		note := c.CollapseNote
		if note == "" {
			note = "(Safe transaction shown above)"
		}
		u.Subsection(fmt.Sprintf("Call  %s  →  %s   %s",
			c.Call.Method, u.Style(c.Call.Destination),
			u.Style(ui.StyledText{Text: note, Severity: ui.SeverityMuted})))
	case c.Call != nil:
		util.PrintFunctionCall(u, c.Call)
	case c.RawData != "":
		u.Subsection("Calldata  " + u.Style(ui.StyledText{Text: "(could not be decoded)", Severity: ui.SeverityWarn}))
		printHexBlock(u.Indent(), c.RawData)
	}

	label := ui.MutedCell
	rows := [][2]ui.TableCell{}
	signer := c.Signer.Text
	if c.Wallet != "" {
		signer += "   " + c.Wallet
	}
	if c.Network != "" {
		signer += "   " + c.Network
	}
	if c.Signer.Text != "" {
		rows = append(rows, [2]ui.TableCell{label("Sign with"), ui.TCS(signer, c.Signer.Severity)})
	} else if c.Network != "" {
		rows = append(rows, [2]ui.TableCell{label("Network"), ui.TC(c.Network)})
	}
	if c.CreateAddress != "" {
		rows = append(rows, [2]ui.TableCell{label("Creates"), ui.TC(c.CreateAddress)})
	} else if c.To.Text != "" {
		toLabel := "Send to"
		switch {
		case c.ToLabel != "":
			toLabel = c.ToLabel
		case c.Safe != nil:
			toLabel = "Safe calls"
		case c.Classic != nil:
			toLabel = "Calls"
		}
		rows = append(rows, [2]ui.TableCell{label(toLabel), ui.TCS(c.To.Text, c.To.Severity)})
	}
	if c.Value != "" {
		rows = append(rows, [2]ui.TableCell{label("Value"), ui.TC(c.Value)})
	}
	if c.Gas != "" {
		gas := c.Gas
		if c.Nonce != "" {
			gas += "   nonce " + c.Nonce
		}
		rows = append(rows, [2]ui.TableCell{label("Gas"), ui.TC(gas)})
	} else if c.Nonce != "" {
		rows = append(rows, [2]ui.TableCell{label("Nonce"), ui.TC(c.Nonce)})
	}
	if s := c.Safe; s != nil {
		if s.Address.Text != "" {
			rows = append(rows, [2]ui.TableCell{label("Safe"), ui.TCS(s.Address.Text, s.Address.Severity)})
		}
		op := ui.TC(s.Operation)
		if s.DelegateCall {
			op = ui.TCS(s.Operation, ui.SeverityWarn)
		}
		if s.Operation != "" {
			rows = append(rows,
				[2]ui.TableCell{label("Operation"), op},
				[2]ui.TableCell{label("Safe nonce"), ui.TC(s.SafeNonce)},
				[2]ui.TableCell{label("safeTxHash"), ui.TC(s.SafeTxHash)},
			)
		}
		if s.Executes != "" {
			rows = append(rows, [2]ui.TableCell{label("Executes"), ui.TC(s.Executes)})
		}
		if s.SafeTxGas != "" {
			rows = append(rows, [2]ui.TableCell{label("Refund"), ui.TCS(fmt.Sprintf(
				"safeTxGas %s · baseGas %s · gasPrice %s · gasToken %s · refundReceiver %s",
				s.SafeTxGas, s.BaseGas, s.GasPrice, s.GasToken, s.RefundReceiver,
			), ui.SeverityMuted)})
		}
	}
	if cl := c.Classic; cl != nil {
		if cl.Multisig.Text != "" {
			rows = append(rows, [2]ui.TableCell{label("Multisig"), ui.TCS(cl.Multisig.Text, cl.Multisig.Severity)})
		}
		if cl.TxID != "" {
			rows = append(rows, [2]ui.TableCell{label("Tx ID"), ui.TC("#" + cl.TxID)})
		}
		status, sev := "pending", ui.SeverityWarn
		switch {
		case cl.Executed:
			status, sev = "executed", ui.SeveritySuccess
		case cl.Threshold > 0 && uint64(cl.Confirmations) >= cl.Threshold:
			status, sev = fmt.Sprintf("ready to execute (%d/%d)", cl.Confirmations, cl.Threshold), ui.SeveritySuccess
		case cl.Threshold > 0:
			status = fmt.Sprintf("pending (%d/%d)", cl.Confirmations, cl.Threshold)
		}
		rows = append(rows, [2]ui.TableCell{label("Status"), ui.TCS(status, sev)})
	}
	if body {
		u.Info("")
	}
	u.KeyValueCells(rows)

	sigs, thresh := []ui.StyledText(nil), uint64(0)
	switch {
	case c.Safe != nil && (len(c.Safe.Signatures) > 0 || c.Safe.Threshold > 0):
		sigs, thresh = c.Safe.Signatures, c.Safe.Threshold
	case c.Classic != nil && (len(c.Classic.Signatures) > 0 || c.Classic.Threshold > 0):
		sigs, thresh = c.Classic.Signatures, c.Classic.Threshold
	}
	if len(sigs) > 0 || thresh > 0 {
		heading := fmt.Sprintf("Signed by (%d)", len(sigs))
		if thresh > 0 {
			heading = fmt.Sprintf("Signed by (%d of %d required)", len(sigs), thresh)
		}
		u.Info("")
		u.Info("%s", u.Style(ui.StyledText{Text: heading, Severity: ui.SeverityMuted}))
		for i, s := range sigs {
			u.Indent().Info("%d. %s", i+1, u.Style(s))
		}
	}

	if len(c.Warnings) > 0 {
		u.Info("")
		for _, w := range c.Warnings {
			u.Warn("! %s", w)
		}
	}
	u.Info("")
}

// ConfirmSigningCard prints the card and asks the card's prompt. It returns
// true when the user accepted (or --yes is set).
func ConfirmSigningCard(u ui.UI, c *SigningCard) bool {
	ShowSigningCard(u, c)
	if config.YesToAllPrompt {
		return true
	}
	return u.Confirm(AnnotateBatch(c.Prompt), true)
}

// printHexBlock writes hex data wrapped to 32-byte words with the byte count
// first, never truncating.
func printHexBlock(u ui.UI, data string) {
	body := strings.TrimPrefix(data, "0x")
	u.Info("%d bytes", len(body)/2)
	util.PrintWrappedHex(u, data)
}

// fillDestinationWarn sets contract/ERC-20 flags and warms token caches so
// later decode and warning text can name the token.
func fillDestinationWarn(warn *WarningInput, to string, network jarvisnetworks.Network) {
	if isContract, err := util.IsContract(to, network); err == nil {
		warn.ToIsContract = isContract
	}
	if isERC20, err := util.IsERC20(to, network); err == nil && isERC20 {
		warn.ToIsERC20 = true
		util.GetERC20Symbol(to, network)
		util.GetERC20Decimal(to, network)
	}
}

// attachMultisigInnerCall puts the decoded inner call (or raw hex, or a
// native Send) on a Classic/Safe card. requireMethod is true for Classic so
// a decode without a method name still shows hex; Safe shows any non-nil
// FunctionCall.
func attachMultisigInnerCall(
	card *SigningCard,
	warn *WarningInput,
	toJarvis jarviscommon.Address,
	value *big.Int,
	data []byte,
	fc *jarviscommon.FunctionCall,
	network jarvisnetworks.Network,
	requireMethod bool,
) {
	warn.Call = fc
	if len(data) > 0 {
		ok := fc != nil
		if requireMethod && ok {
			ok = fc.Method != ""
		}
		if ok {
			card.Call = util.NewFunctionCallDisplay(fc, network)
		} else {
			card.RawData = "0x" + ethcommon.Bytes2Hex(data)
		}
	} else if value != nil && value.Sign() > 0 {
		card.Call = util.NewFunctionCallDisplay(&jarviscommon.FunctionCall{
			Destination: toJarvis,
			Value:       value,
		}, network)
		if !warn.ToIsContract {
			card.ToLabel = "Recipient"
		}
	}
	card.Warnings = SigningWarnings(*warn)
}

// WarningInput is what SigningWarnings looks at. It is deliberately a plain
// struct so the derivation can be unit-tested without a chain.
type WarningInput struct {
	To           jarviscommon.Address
	ToIsContract bool
	Value        *big.Int
	NativeSymbol string
	// NativeDecimals is the native token's decimal count; 0 means 18.
	NativeDecimals uint64
	HasData        bool
	Call           *jarviscommon.FunctionCall
	// ToIsERC20 is true when the destination is an ERC-20 token contract.
	ToIsERC20    bool
	DelegateCall bool
	MultiSend    bool
	// SignerBalance and MaxCost enable the insufficient-funds warning; either
	// nil skips it. MaxCost is value + gasLimit × max fee.
	SignerBalance *big.Int
	MaxCost       *big.Int
}

func (in WarningInput) nativeDecimals() uint64 {
	if in.NativeDecimals == 0 {
		return 18
	}
	return in.NativeDecimals
}

func (in WarningInput) nativeAmount(v *big.Int) string {
	return jarviscommon.CompactAmount(jarviscommon.BigToFloatString(v, in.nativeDecimals()))
}

// approvalMethods maps ERC-20/721/1155 approval selectors to the parameter
// names that carry the spender/operator and the amount.
var approvalMethods = map[string]struct{ spender, amount string }{
	"approve":           {"spender", "amount"},
	"increaseAllowance": {"spender", "addedValue"},
	"setApprovalForAll": {"operator", "approved"},
	"permit":            {"spender", "value"},
}

// SigningWarnings derives the yellow "!" lines from a transaction. Each
// warning is one sentence stating a fact the reader may not have noticed in
// the decoded call; the list is empty for a routine transaction.
func SigningWarnings(in WarningInput) []string {
	var out []string

	if in.To.Address != "" && !jarviscommon.IsKnownAddress(in.To) {
		out = append(out, fmt.Sprintf("%s is not in your address book", in.To.Address))
	}
	if w := erc20NativeValueWarnings(in.Call, in.To, in.ToIsERC20, in.Value, in.NativeSymbol, in.nativeDecimals()); len(w) > 0 {
		out = append(out, w...)
	} else if in.Value != nil && in.Value.Sign() > 0 && in.ToIsContract {
		out = append(out, fmt.Sprintf("sends %s %s into a contract",
			in.nativeAmount(in.Value), in.NativeSymbol))
	}
	if in.SignerBalance != nil && in.MaxCost != nil && in.SignerBalance.Cmp(in.MaxCost) < 0 {
		out = append(out, fmt.Sprintf("balance %s %s does not cover value + max gas (%s %s); the tx would be rejected",
			in.nativeAmount(in.SignerBalance), in.NativeSymbol,
			in.nativeAmount(in.MaxCost), in.NativeSymbol))
	}
	if in.DelegateCall {
		if in.MultiSend {
			out = append(out, "DELEGATECALL into MultiSend: every inner call below runs with the Safe's full authority")
		} else {
			out = append(out, "DELEGATECALL: the target's code runs in the Safe's own context")
		}
	}
	if in.HasData && (in.Call == nil || in.Call.Method == "") {
		dest := in.To.Address
		if in.Call != nil && in.Call.Destination.Address != "" {
			dest = in.Call.Destination.Address
		}
		out = append(out, fmt.Sprintf("calldata could not be decoded (no ABI for %s); review the raw bytes", dest))
	}
	if in.Call != nil {
		out = append(out, approvalWarnings(in.Call)...)
	}
	return out
}

func approvalWarnings(fc *jarviscommon.FunctionCall) []string {
	var out []string
	if spec, ok := approvalMethods[fc.Method]; ok {
		var spender *jarviscommon.Address
		var amount *jarviscommon.Value
		for i := range fc.Params {
			p := &fc.Params[i]
			if len(p.Values) != 1 {
				continue
			}
			switch {
			case p.Values[0].Kind == jarviscommon.DisplayAddress && (strings.EqualFold(p.Name, spec.spender) || spender == nil && strings.Contains(strings.ToLower(p.Name), "spender")):
				spender = p.Values[0].Address
			case strings.EqualFold(p.Name, spec.amount) || (amount == nil && p.Values[0].Kind != jarviscommon.DisplayAddress):
				amount = &p.Values[0]
			}
		}
		target := fc.Destination.Address
		if jarviscommon.IsKnownAddress(fc.Destination) {
			target = fc.Destination.Desc
		}
		spenderText := "an unspecified spender"
		if spender != nil {
			spenderText = jarviscommon.PlainAddress(*spender)
		}
		switch {
		case fc.Method == "setApprovalForAll" && amount != nil && amount.Raw == "true":
			out = append(out, fmt.Sprintf("grants %s control over ALL %s tokens", spenderText, target))
		case amount != nil && isMaxUint(amount.Raw):
			out = append(out, fmt.Sprintf("approves UNLIMITED %s to %s", target, spenderText))
		}
		if spender != nil && !jarviscommon.IsKnownAddress(*spender) {
			out = append(out, fmt.Sprintf("spender %s is not in your address book", spender.Address))
		}
	}
	for _, inner := range fc.DecodedFunctionCalls {
		out = append(out, approvalWarnings(inner)...)
	}
	return out
}

func isMaxUint(raw string) bool {
	_, ok := jarviscommon.MaxUintLabel(raw)
	return ok
}

func erc20NativeValueWarnings(
	fc *jarviscommon.FunctionCall,
	dest jarviscommon.Address,
	destIsERC20 bool,
	value *big.Int,
	symbol string,
	decimals uint64,
) []string {
	var out []string
	if w := oneERC20NativeValueWarning(fc, dest, destIsERC20, value, symbol, decimals); w != "" {
		out = append(out, w)
	}
	if fc == nil {
		return out
	}
	for _, inner := range fc.DecodedFunctionCalls {
		out = append(out, erc20NativeValueWarnings(
			inner, inner.Destination, isERC20Write(inner.Method), inner.Value, symbol, decimals,
		)...)
	}
	return out
}

func oneERC20NativeValueWarning(
	fc *jarviscommon.FunctionCall,
	dest jarviscommon.Address,
	destIsERC20 bool,
	value *big.Int,
	symbol string,
	decimals uint64,
) string {
	if value == nil || value.Sign() <= 0 {
		return ""
	}
	method := ""
	if fc != nil {
		method = fc.Method
	}
	if isPayableTokenMethod(method) {
		return ""
	}
	if !destIsERC20 && !isERC20Write(method) {
		return ""
	}
	token := dest.Address
	if jarviscommon.IsKnownAddress(dest) {
		if label := strings.TrimSpace(strings.TrimSuffix(dest.Desc, " token")); label != "" {
			token = label
		}
	}
	amount := jarviscommon.CompactAmount(jarviscommon.BigToFloatString(value, decimals))
	what := "call"
	if method != "" {
		what = method
	}
	return fmt.Sprintf("attaches %s %s to an ERC-20 %s on %s; the native value goes to the token contract, not the recipient",
		amount, symbol, what, token)
}

func isERC20Write(method string) bool {
	switch method {
	case "transfer", "transferFrom", "approve", "increaseAllowance", "decreaseAllowance", "permit":
		return true
	}
	return false
}

func isPayableTokenMethod(method string) bool {
	switch method {
	case "deposit", "depositTo":
		return true
	}
	return false
}

// FormatGasLine renders the fee parameters of a tx in one line, cost first:
// "≈ 0.00170246 ETH   (85,123 gas × max 20 gwei, tip 1.5 gwei)".
func FormatGasLine(legacy bool, gasPrice, feeCap, tipCap *big.Int, gasLimit uint64, symbol string) string {
	price := feeCap
	if legacy {
		price = gasPrice
	}
	cost := jarviscommon.BigToFloat(MaxGasCost(price, gasLimit), 18)
	gas := jarviscommon.GroupDigits(fmt.Sprintf("%d", gasLimit))
	if legacy {
		return fmt.Sprintf("≈ %.8f %s   (%s gas × %s gwei)", cost, symbol, gas, gweiText(gasPrice))
	}
	return fmt.Sprintf("≈ %.8f %s   (%s gas × max %s gwei, tip %s gwei)",
		cost, symbol, gas, gweiText(feeCap), gweiText(tipCap))
}

// MaxGasCost is gasLimit × price: the most the fee can come to.
func MaxGasCost(price *big.Int, gasLimit uint64) *big.Int {
	if price == nil {
		return new(big.Int)
	}
	return new(big.Int).Mul(new(big.Int).SetUint64(gasLimit), price)
}

// gweiText renders a wei amount in gwei with up to four decimals and no
// trailing zeros: 20 → "20", 1.5 → "1.5", 0.01234 → "0.0123".
func gweiText(wei *big.Int) string {
	s := fmt.Sprintf("%.4f", jarviscommon.BigToFloat(wei, 9))
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// ExplainEstimateGasError turns the multi-node error from a failed gas
// estimation into one sentence the operator can act on. Nodes disagree on
// wording, so the classification is by keyword; anything unrecognised is
// reported as the first node's message.
func ExplainEstimateGasError(err error, from string, balance *big.Int, symbol string) string {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "insufficient funds") || strings.Contains(lower, "outoffunds") || strings.Contains(lower, "out of funds"):
		have := "no"
		if balance != nil {
			have = jarviscommon.BigToFloatString(balance, 18)
		}
		return fmt.Sprintf("Gas estimation failed: %s holds %s %s, not enough for the value plus gas. Fund the wallet or lower the amount.",
			from, have, symbol)
	case strings.Contains(lower, "execution reverted") || strings.Contains(lower, "revert"):
		reason := ""
		if i := strings.Index(lower, "revert"); i >= 0 {
			rest := strings.TrimSpace(msg[i+len("revert"):])
			rest = strings.TrimPrefix(strings.TrimPrefix(rest, "ed"), ":")
			if line := strings.SplitN(strings.TrimSpace(rest), "\n", 2)[0]; line != "" {
				reason = " (" + line + ")"
			}
		}
		return "Gas estimation failed: the call reverts against the current chain state" + reason +
			". The transaction would fail if sent; check the parameters or pass -g <gas limit> to skip estimation."
	}
	first := strings.SplitN(strings.TrimPrefix(msg, "couldn't read from any nodes: "), "\n", 2)[0]
	return "Gas estimation failed: " + first
}
