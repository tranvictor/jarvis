package util

import (
	"fmt"
	"math/big"
	"strings"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
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
	Kind    string // "EOA transaction", "Safe proposal", "Safe approval", "Safe execution"
	Network string

	Signer ui.StyledText
	To     ui.StyledText // empty Text for contract creation; see CreateAddress
	// CreateAddress is the predicted address when the tx deploys a contract.
	CreateAddress string
	Value         string // "1.5 ETH"; empty hides the row
	Gas           string // "max 20.0 gwei, tip 1.5 gwei · 85,123 gas · ≈ 0.0017 ETH"; empty hides
	Nonce         string

	Safe *SafeCardFields

	// Call is the decoded calldata. Nil when there is none or it could not be
	// analyzed; RawData is shown instead when set.
	Call    *util.FunctionCallDisplay
	RawData string
	// CollapseCall prints the call as a single header line. Used when the
	// same call was fully displayed moments ago (Safe approve → execute).
	CollapseCall bool

	// ClearSign renders the ERC-7730 view when a descriptor matched. It is
	// a callback so this package does not depend on the erc7730 engine.
	ClearSign func(ui.UI)

	Warnings []string
	Prompt   string
}

// SafeCardFields are the SafeTx parameters that have no EOA equivalent.
type SafeCardFields struct {
	Operation      string // "CALL" or "DELEGATECALL"
	DelegateCall   bool
	MultiSend      bool
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

// ShowSigningCard prints the card. Order is chosen so that what the reader
// must verify sits directly above the prompt: the call first, then the
// summary block, then the warnings.
func ShowSigningCard(u ui.UI, c *SigningCard) {
	u.Section(c.Kind)

	if c.ClearSign != nil {
		c.ClearSign(u)
	}

	switch {
	case c.Call != nil && c.CollapseCall:
		u.Subsection(fmt.Sprintf("Call  %s  →  %s   %s",
			c.Call.Method, u.Style(c.Call.Destination),
			u.Style(ui.StyledText{Text: "(Safe transaction shown above)", Severity: ui.SeverityMuted})))
	case c.Call != nil:
		util.PrintFunctionCall(u, c.Call)
	case c.RawData != "":
		u.Subsection("Calldata  " + u.Style(ui.StyledText{Text: "(could not be decoded)", Severity: ui.SeverityWarn}))
		printHexBlock(u.Indent(), c.RawData)
	}

	label := func(s string) ui.TableCell { return ui.TCS(s, ui.SeverityMuted) }
	rows := [][2]ui.TableCell{}
	signer := c.Signer.Text
	if c.Network != "" {
		signer += "   " + c.Network
	}
	if c.Signer.Text != "" {
		rows = append(rows, [2]ui.TableCell{label("Sign with"), ui.TCS(signer, c.Signer.Severity)})
	}
	if c.CreateAddress != "" {
		rows = append(rows, [2]ui.TableCell{label("Creates"), ui.TC(c.CreateAddress)})
	} else if c.To.Text != "" {
		toLabel := "Send to"
		if c.Safe != nil {
			toLabel = "Safe calls"
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
	u.Info("")
	u.KeyValueCells(rows)

	if c.Safe != nil && (len(c.Safe.Signatures) > 0 || c.Safe.Threshold > 0) {
		heading := fmt.Sprintf("Signed by (%d)", len(c.Safe.Signatures))
		if c.Safe.Threshold > 0 {
			heading = fmt.Sprintf("Signed by (%d of %d required)", len(c.Safe.Signatures), c.Safe.Threshold)
		}
		u.Info("")
		u.Info("%s", u.Style(ui.StyledText{Text: heading, Severity: ui.SeverityMuted}))
		for i, s := range c.Safe.Signatures {
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
	return u.Confirm(c.Prompt, true)
}

// printHexBlock writes hex data wrapped to 32-byte words with the byte count
// first, never truncating.
func printHexBlock(u ui.UI, data string) {
	body := strings.TrimPrefix(data, "0x")
	u.Info("%d bytes", len(body)/2)
	const width = 64
	for i := 0; i < len(body); i += width {
		end := i + width
		if end > len(body) {
			end = len(body)
		}
		prefix := "  "
		if i == 0 {
			prefix = "0x"
		}
		u.Info("%s%s", prefix, body[i:end])
	}
}

// WarningInput is what SigningWarnings looks at. It is deliberately a plain
// struct so the derivation can be unit-tested without a chain.
type WarningInput struct {
	To           jarviscommon.Address
	ToIsContract bool
	Value        *big.Int
	NativeSymbol string
	HasData      bool
	Call         *jarviscommon.FunctionCall
	DelegateCall bool
	MultiSend    bool
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
	if in.Value != nil && in.Value.Sign() > 0 && in.ToIsContract {
		out = append(out, fmt.Sprintf("sends %s %s into a contract",
			jarviscommon.BigToFloatString(in.Value, 18), in.NativeSymbol))
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

// FormatGasLine renders the fee parameters of a tx in one line:
// "max 20.0000 gwei, tip 1.5000 gwei · 85123 gas · ≈ 0.00170246 ETH".
func FormatGasLine(legacy bool, gasPrice, feeCap, tipCap *big.Int, gasLimit uint64, symbol string) string {
	price := feeCap
	if legacy {
		price = gasPrice
	}
	cost := jarviscommon.BigToFloat(new(big.Int).Mul(new(big.Int).SetUint64(gasLimit), price), 18)
	if legacy {
		return fmt.Sprintf("%.4f gwei · %d gas · ≈ %.8f %s",
			jarviscommon.BigToFloat(gasPrice, 9), gasLimit, cost, symbol)
	}
	return fmt.Sprintf("max %.4f gwei, tip %.4f gwei · %d gas · ≈ %.8f %s",
		jarviscommon.BigToFloat(feeCap, 9), jarviscommon.BigToFloat(tipCap, 9), gasLimit, cost, symbol)
}
