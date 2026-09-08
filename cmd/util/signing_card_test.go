package util

import (
	"math/big"
	"strings"
	"testing"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
)

const (
	cardMe     = "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"
	cardRouter = "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"
	cardUSDC   = "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"
	maxUint    = "115792089237316195423570985008687907853269984665640564039457584007913129639935"
)

func cardAddr(hex, desc string) jarviscommon.Address {
	return jarviscommon.Address{Address: hex, Desc: desc}
}

func hasEntry(rec *ui.RecordingUI, method, value string) bool {
	for _, e := range rec.Entries() {
		if e.Method == method && e.Value == value {
			return true
		}
	}
	return false
}

func addrParam(name, hex, desc string) jarviscommon.ParamResult {
	a := cardAddr(hex, desc)
	return jarviscommon.ParamResult{Name: name, Type: "address", Values: []jarviscommon.Value{
		{Raw: hex, Kind: jarviscommon.DisplayAddress, Address: &a},
	}}
}

func uintParam(name, raw string) jarviscommon.ParamResult {
	return jarviscommon.ParamResult{Name: name, Type: "uint256", Values: []jarviscommon.Value{
		{Raw: raw, Kind: jarviscommon.DisplayInteger},
	}}
}

func TestSigningWarningsRoutineTxHasNone(t *testing.T) {
	got := SigningWarnings(WarningInput{
		To: cardAddr(cardRouter, "Uniswap V2 Router"), ToIsContract: true, Value: big.NewInt(0),
		HasData: true,
		Call: &jarviscommon.FunctionCall{
			Destination: cardAddr(cardRouter, "Uniswap V2 Router"), Method: "swapExactTokensForTokens",
		},
	})
	if len(got) != 0 {
		t.Fatalf("expected no warnings, got %v", got)
	}
}

func TestSigningWarningsCoverTheRiskyCases(t *testing.T) {
	cases := []struct {
		name string
		in   WarningInput
		want []string
	}{
		{
			name: "unknown destination",
			in:   WarningInput{To: cardAddr(cardRouter, "unknown")},
			want: []string{cardRouter + " is not in your address book"},
		},
		{
			name: "value into a contract",
			in: WarningInput{
				To: cardAddr(cardRouter, "Router"), ToIsContract: true,
				Value: big.NewInt(1500000000000000000), NativeSymbol: "ETH",
			},
			want: []string{"sends 1.5 ETH into a contract"},
		},
		{
			name: "erc20 call with native value",
			in: WarningInput{
				To: cardAddr(cardUSDC, "USDC"), ToIsContract: true, ToIsERC20: true,
				Value: big.NewInt(1500000000000000000), NativeSymbol: "ETH",
				Call: &jarviscommon.FunctionCall{
					Destination: cardAddr(cardUSDC, "USDC"), Method: "transfer",
					Params: []jarviscommon.ParamResult{addrParam("to", cardMe, "me"), uintParam("amount", "1000")},
				},
			},
			want: []string{"attaches 1.5 ETH to an ERC-20 transfer on USDC"},
		},
		{
			name: "weth deposit keeps the generic contract-value warning",
			in: WarningInput{
				To: cardAddr(cardUSDC, "WETH"), ToIsContract: true, ToIsERC20: true,
				Value: big.NewInt(1500000000000000000), NativeSymbol: "ETH",
				Call: &jarviscommon.FunctionCall{
					Destination: cardAddr(cardUSDC, "WETH"), Method: "deposit",
				},
			},
			want: []string{"sends 1.5 ETH into a contract"},
		},
		{
			name: "delegatecall multisend",
			in:   WarningInput{To: cardAddr(cardRouter, "MultiSendCallOnly"), DelegateCall: true, MultiSend: true},
			want: []string{"DELEGATECALL into MultiSend"},
		},
		{
			name: "undecoded calldata",
			in:   WarningInput{To: cardAddr(cardRouter, "Router"), HasData: true, Call: &jarviscommon.FunctionCall{Destination: cardAddr(cardRouter, "Router")}},
			want: []string{"calldata could not be decoded (no ABI for " + cardRouter + ")"},
		},
		{
			name: "unlimited approve",
			in: WarningInput{To: cardAddr(cardUSDC, "USDC"), Call: &jarviscommon.FunctionCall{
				Destination: cardAddr(cardUSDC, "USDC"), Method: "approve",
				Params: []jarviscommon.ParamResult{addrParam("spender", cardRouter, "Uniswap V2 Router"), uintParam("amount", maxUint)},
			}},
			want: []string{"approves UNLIMITED USDC to " + cardRouter + " (Uniswap V2 Router)"},
		},
		{
			name: "approve to unknown spender",
			in: WarningInput{To: cardAddr(cardUSDC, "USDC"), Call: &jarviscommon.FunctionCall{
				Destination: cardAddr(cardUSDC, "USDC"), Method: "approve",
				Params: []jarviscommon.ParamResult{addrParam("spender", cardRouter, ""), uintParam("amount", "1000")},
			}},
			want: []string{"spender " + cardRouter + " is not in your address book"},
		},
		{
			name: "setApprovalForAll inside a multisend",
			in: WarningInput{To: cardAddr(cardRouter, "MultiSend"), Call: &jarviscommon.FunctionCall{
				Destination: cardAddr(cardRouter, "MultiSend"), Method: "multiSend",
				DecodedFunctionCalls: []*jarviscommon.FunctionCall{{
					Destination: cardAddr(cardUSDC, "CoolNFT"), Method: "setApprovalForAll",
					Params: []jarviscommon.ParamResult{
						addrParam("operator", cardMe, "me"),
						{Name: "approved", Type: "bool", Values: []jarviscommon.Value{{Raw: "true", Kind: jarviscommon.DisplayRaw}}},
					},
				}},
			}},
			want: []string{"grants " + cardMe + " (me) control over ALL CoolNFT tokens"},
		},
	}
	for _, c := range cases {
		got := SigningWarnings(c.in)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %d warnings %v, want %d", c.name, len(got), got, len(c.want))
			continue
		}
		for i := range c.want {
			if !strings.Contains(got[i], c.want[i]) {
				t.Errorf("%s: warning %d = %q, want to contain %q", c.name, i, got[i], c.want[i])
			}
		}
	}
}

func TestShowSigningCardOrderAndFullAddresses(t *testing.T) {
	rec := ui.NewRecordingUI("y")
	fc := &jarviscommon.FunctionCall{
		Destination: cardAddr(cardUSDC, "USDC"), Method: "approve",
		Params: []jarviscommon.ParamResult{addrParam("spender", cardRouter, ""), uintParam("amount", maxUint)},
	}
	card := &SigningCard{
		Kind:     "EOA transaction",
		Network:  "mainnet",
		Signer:   util.StyledAddress(cardAddr(cardMe, "me")),
		To:       util.StyledAddress(cardAddr(cardUSDC, "USDC")),
		Gas:      "max 20.0000 gwei, tip 1.5000 gwei · 85123 gas · ≈ 0.00170246 ETH",
		Nonce:    "42",
		Call:     util.NewFunctionCallDisplay(fc, nil),
		Warnings: SigningWarnings(WarningInput{To: cardAddr(cardUSDC, "USDC"), HasData: true, Call: fc}),
		Prompt:   "Sign and broadcast (≈ 0.00170246 ETH)?",
	}
	if !ConfirmSigningCard(rec, card) {
		t.Fatal("scripted 'y' should confirm")
	}

	var got []string
	for _, e := range rec.Entries() {
		got = append(got, e.Method+": "+e.Value)
	}
	joined := strings.Join(got, "\n")

	// Order: title, call, summary, warnings, prompt.
	order := []string{
		"Section: EOA transaction",
		"Subsection: Call  approve  →  " + cardUSDC + " (USDC)",
		"Info: spender  " + cardRouter + "  address",
		"KeyValue: Sign with: " + cardMe + " (me)   mainnet",
		"KeyValue: Send to: " + cardUSDC + " (USDC)",
		"KeyValue: Gas: max 20.0000 gwei, tip 1.5000 gwei · 85123 gas · ≈ 0.00170246 ETH   nonce 42",
		"Warn: ! approves UNLIMITED USDC to " + cardRouter,
		"Confirm: Sign and broadcast (≈ 0.00170246 ETH)?",
	}
	pos := -1
	for _, w := range order {
		idx := strings.Index(joined, w)
		if idx < 0 {
			t.Fatalf("missing %q in:\n%s", w, joined)
		}
		if idx < pos {
			t.Fatalf("%q out of order in:\n%s", w, joined)
		}
		pos = idx
	}
	if strings.Contains(joined, "…") {
		t.Fatalf("signing card must never shorten addresses:\n%s", joined)
	}
}

func TestShowSigningCardSafeFieldsAndCollapse(t *testing.T) {
	rec := ui.NewRecordingUI()
	card := &SigningCard{
		Kind:   "Safe approval",
		Signer: util.StyledAddress(cardAddr(cardMe, "me")),
		To:     util.StyledAddress(cardAddr(cardRouter, "MultiSendCallOnly")),
		Safe: &SafeCardFields{
			Operation: "DELEGATECALL (1)", DelegateCall: true, MultiSend: true,
			SafeNonce: "17", SafeTxHash: "0xabc",
			SafeTxGas: "0", BaseGas: "0", GasPrice: "0", GasToken: "0x0", RefundReceiver: "0x0",
			Signatures: []ui.StyledText{{Text: "[off-chain] " + cardMe + " (me)"}},
			Threshold:  2,
		},
		Warnings: SigningWarnings(WarningInput{To: cardAddr(cardRouter, "MultiSendCallOnly"), DelegateCall: true, MultiSend: true}),
	}
	ShowSigningCard(rec, card)
	if !hasEntry(rec, "BoxedSection", "Safe approval") {
		t.Fatalf("Safe approval must be boxed: %v", rec.Entries())
	}
	if hasEntry(rec, "Section", "Safe approval") {
		t.Fatalf("Safe approval must not also use a Section rule: %v", rec.Entries())
	}
	for _, w := range []string{
		"Safe calls: " + cardRouter + " (MultiSendCallOnly)",
		"Operation: DELEGATECALL (1)",
		"Safe nonce: 17",
		"safeTxHash: 0xabc",
		"Signed by (1 of 2 required)",
		"1. [off-chain] " + cardMe + " (me)",
		"! DELEGATECALL into MultiSend",
	} {
		if !rec.HasMessage(w) {
			t.Fatalf("missing %q in %v", w, rec.Entries())
		}
	}

	rec = ui.NewRecordingUI()
	exec := &SigningCard{
		Kind:         "Safe execution",
		Call:         util.NewFunctionCallDisplay(&jarviscommon.FunctionCall{Destination: cardAddr(cardRouter, "Safe"), Method: "execTransaction", Params: []jarviscommon.ParamResult{uintParam("value", "0")}}, nil),
		CollapseCall: true,
		Safe:         &SafeCardFields{Executes: "0xabc"},
	}
	ShowSigningCard(rec, exec)
	if hasEntry(rec, "Section", "Safe execution") || hasEntry(rec, "BoxedSection", "Safe execution") {
		t.Fatalf("Safe execution wrapper must be a quiet heading, not a section or box: %v", rec.Entries())
	}
	if !hasEntry(rec, "Subsection", "Safe execution") {
		t.Fatalf("Safe execution wrapper heading missing: %v", rec.Entries())
	}
	if !rec.HasMessage("Call  execTransaction  →  " + cardRouter + " (Safe)   (Safe transaction shown above)") {
		t.Fatalf("collapsed call header missing: %v", rec.Entries())
	}
	if rec.HasMessage("value  0") {
		t.Fatalf("collapsed call must not print params: %v", rec.Entries())
	}
	if !rec.HasMessage("Executes: 0xabc") || rec.HasMessage("Operation:") {
		t.Fatalf("execution card should link the safeTxHash without repeating Safe params: %v", rec.Entries())
	}
}

func TestFormatGasLine(t *testing.T) {
	gwei := big.NewInt(1_000_000_000)
	legacy := FormatGasLine(true, new(big.Int).Mul(big.NewInt(20), gwei), nil, nil, 21000, "ETH")
	if legacy != "≈ 0.00042000 ETH   (21,000 gas × 20 gwei)" {
		t.Fatalf("legacy: %q", legacy)
	}
	tip := new(big.Int).Div(new(big.Int).Mul(big.NewInt(15), gwei), big.NewInt(10)) // 1.5 gwei
	dyn := FormatGasLine(false, nil, new(big.Int).Mul(big.NewInt(20), gwei), tip, 85123, "ETH")
	if dyn != "≈ 0.00170246 ETH   (85,123 gas × max 20 gwei, tip 1.5 gwei)" {
		t.Fatalf("dynamic: %q", dyn)
	}
	if gasCostOnly(dyn) != "≈ 0.00170246 ETH" {
		t.Fatalf("gasCostOnly: %q", gasCostOnly(dyn))
	}
}

func TestInsufficientBalanceWarning(t *testing.T) {
	eth := big.NewInt(1_000_000_000_000_000_000)
	in := WarningInput{
		To:            jarviscommon.Address{Address: cardMe, Desc: "me"},
		Value:         new(big.Int).Mul(big.NewInt(2), eth),
		NativeSymbol:  "ETH",
		SignerBalance: eth,
		MaxCost:       new(big.Int).Add(new(big.Int).Mul(big.NewInt(2), eth), big.NewInt(420_000_000_000_000)),
	}
	got := SigningWarnings(in)
	if len(got) != 1 || got[0] != "balance 1 ETH does not cover value + max gas (2.0004 ETH); the tx would be rejected" {
		t.Fatalf("warnings: %q", got)
	}
	in.SignerBalance = new(big.Int).Mul(big.NewInt(3), eth)
	if got := SigningWarnings(in); len(got) != 0 {
		t.Fatalf("sufficient balance must not warn: %q", got)
	}
	in.SignerBalance = nil
	if got := SigningWarnings(in); len(got) != 0 {
		t.Fatalf("unknown balance must not warn: %q", got)
	}
}

func TestSigningWarningsHonourNativeDecimals(t *testing.T) {
	// 8-decimal native: 1.5 units into a contract must not be rendered as if 18.
	in := WarningInput{
		To: cardAddr(cardRouter, "Router"), ToIsContract: true,
		Value: big.NewInt(150_000_000), NativeSymbol: "TKN", NativeDecimals: 8,
	}
	got := SigningWarnings(in)
	if len(got) != 1 || got[0] != "sends 1.5 TKN into a contract" {
		t.Fatalf("8-decimal native: %q", got)
	}
}

func TestShowSigningCardClassicFields(t *testing.T) {
	rec := ui.NewRecordingUI()
	fc := &jarviscommon.FunctionCall{
		Destination: cardAddr(cardUSDC, "USDC"), Method: "transfer",
		Params: []jarviscommon.ParamResult{addrParam("to", cardMe, "me"), uintParam("amount", "1000")},
	}
	ShowSigningCard(rec, &SigningCard{
		Kind:    "Classic multisig transaction",
		Network: "mainnet",
		To:      util.StyledAddress(cardAddr(cardUSDC, "USDC")),
		Value:   "1.5 ETH",
		Call:    util.NewFunctionCallDisplay(fc, nil),
		Classic: &ClassicCardFields{
			TxID:          "42",
			Multisig:      util.StyledAddress(cardAddr(cardMe, "Treasury")),
			Confirmations: 1,
			Threshold:     2,
			Signatures:    []ui.StyledText{util.StyledAddress(cardAddr(cardMe, "me"))},
		},
	})
	if !hasEntry(rec, "BoxedSection", "Classic multisig transaction") {
		t.Fatalf("Classic inner tx must be boxed: %v", rec.Entries())
	}
	if hasEntry(rec, "Section", "Classic multisig transaction") {
		t.Fatalf("Classic inner tx must not use a Section rule: %v", rec.Entries())
	}
	for _, w := range []string{
		"Classic multisig transaction",
		"Send  1000  →  " + cardMe + " (me)",
		"Calls: " + cardUSDC + " (USDC)",
		"Multisig: " + cardMe + " (Treasury)",
		"Tx ID: #42",
		"Status: pending (1/2)",
		"Network: mainnet",
		"Signed by (1 of 2 required)",
		"1. " + cardMe + " (me)",
	} {
		if !rec.HasMessage(w) {
			t.Fatalf("missing %q in %v", w, rec.Entries())
		}
	}
	if rec.HasMessage("Safe calls:") {
		t.Fatalf("Classic card must not use the Safe destination label: %v", rec.Entries())
	}
}

func TestShowSigningCardClassicCollapseNote(t *testing.T) {
	rec := ui.NewRecordingUI()
	ShowSigningCard(rec, &SigningCard{
		Kind: "EOA transaction",
		Call: util.NewFunctionCallDisplay(&jarviscommon.FunctionCall{
			Destination: cardAddr(cardMe, "Treasury"), Method: "confirmTransaction",
			Params: []jarviscommon.ParamResult{uintParam("transactionId", "42")},
		}, nil),
		CollapseCall: true,
		CollapseNote: "(Classic transaction shown above)",
	})
	if !rec.HasMessage("Call  confirmTransaction  →  " + cardMe + " (Treasury)   (Classic transaction shown above)") {
		t.Fatalf("collapsed Classic note missing: %v", rec.Entries())
	}
	if hasEntry(rec, "Section", "EOA transaction") || hasEntry(rec, "BoxedSection", "EOA transaction") {
		t.Fatalf("EOA confirm wrapper must be a quiet heading: %v", rec.Entries())
	}
	if !hasEntry(rec, "Subsection", "EOA transaction") {
		t.Fatalf("EOA confirm wrapper heading missing: %v", rec.Entries())
	}
	if rec.HasMessage("transactionId  42") {
		t.Fatalf("collapsed call must not print params: %v", rec.Entries())
	}
}

func TestShowSigningCardClassicProposalOmitsTxID(t *testing.T) {
	rec := ui.NewRecordingUI()
	ShowSigningCard(rec, &SigningCard{
		Kind: "Classic multisig transaction",
		To:   util.StyledAddress(cardAddr(cardUSDC, "USDC")),
		Classic: &ClassicCardFields{
			Multisig:  util.StyledAddress(cardAddr(cardMe, "Treasury")),
			Threshold: 2,
		},
	})
	if rec.HasMessage("Tx ID:") {
		t.Fatalf("proposal card must not invent a tx id: %v", rec.Entries())
	}
	if !rec.HasMessage("Classic multisig transaction") || !rec.HasMessage("Multisig: "+cardMe+" (Treasury)") {
		t.Fatalf("proposal card missing: %v", rec.Entries())
	}
}

func TestShowSigningCardNativeSendToEOA(t *testing.T) {
	rec := ui.NewRecordingUI()
	fc := &jarviscommon.FunctionCall{
		Destination: cardAddr(cardMe, "Alice"),
		Value:       big.NewInt(1_500_000_000_000_000_000),
	}
	ShowSigningCard(rec, &SigningCard{
		Kind:    "Safe approval",
		Network: "mainnet",
		To:      util.StyledAddress(cardAddr(cardMe, "Alice")),
		ToLabel: "Recipient",
		Value:   "1.5 ETH",
		Call:    util.NewFunctionCallDisplay(fc, nil),
		Safe: &SafeCardFields{
			Operation: "CALL (0)", SafeNonce: "3", SafeTxHash: "0xabc",
		},
	})
	if !rec.HasMessage("Send  1.5 ETH  →  " + cardMe + " (Alice)") {
		t.Fatalf("native send headline missing: %v", rec.Entries())
	}
	if !rec.HasMessage("Recipient: " + cardMe + " (Alice)") {
		t.Fatalf("EOA send should label the destination Recipient: %v", rec.Entries())
	}
	if rec.HasMessage("Safe calls:") {
		t.Fatalf("EOA send must not look like a contract call: %v", rec.Entries())
	}
	if rec.HasMessage("<undecoded>") {
		t.Fatalf("native send must not look undecoded: %v", rec.Entries())
	}
}

func TestShowSigningCardERC20TransferHeadline(t *testing.T) {
	rec := ui.NewRecordingUI()
	alice := cardAddr(cardMe, "Alice")
	amount := jarviscommon.Value{
		Raw: "1000000000", Kind: jarviscommon.DisplayToken,
		Token: &jarviscommon.TokenHint{Decimal: 6, Symbol: "USDC"},
	}
	fc := &jarviscommon.FunctionCall{
		Destination: cardAddr(cardUSDC, "USDC token"), Method: "transfer",
		Params: []jarviscommon.ParamResult{
			addrParam("_to", cardMe, "Alice"),
			{Name: "_value", Type: "uint256", Values: []jarviscommon.Value{amount}},
		},
	}
	ShowSigningCard(rec, &SigningCard{
		Kind: "Safe approval",
		To:   util.StyledAddress(cardAddr(cardUSDC, "USDC")),
		Call: util.NewFunctionCallDisplay(fc, nil),
		Safe: &SafeCardFields{Operation: "CALL (0)", SafeNonce: "4", SafeTxHash: "0xabc"},
	})
	if !rec.HasMessage("Send  1,000 USDC  →  " + alice.Address + " (Alice)") {
		t.Fatalf("erc20 send headline missing: %v", rec.Entries())
	}
	if !rec.HasMessage("Safe calls: " + cardUSDC + " (USDC)") {
		t.Fatalf("token destination should stay on Safe calls: %v", rec.Entries())
	}
}

func TestSigningCardShowsWalletKind(t *testing.T) {
	rec := ui.NewRecordingUI()
	ShowSigningCard(rec, &SigningCard{
		Kind:    "EOA transaction",
		Network: "mainnet",
		Signer:  ui.StyledText{Text: cardMe + " (hot wallet)"},
		Wallet:  "ledger",
		To:      ui.StyledText{Text: cardUSDC + " (USDC)"},
	})
	if !rec.HasMessage("Sign with: " + cardMe + " (hot wallet)   ledger   mainnet") {
		t.Fatalf("wallet kind missing from signer line: %v", rec.Entries())
	}
}

func TestShowSigningCardBatchMark(t *testing.T) {
	t.Cleanup(ClearBatchItem)
	SetBatchItem(12, 87)
	rec := ui.NewRecordingUI("y")
	card := &SigningCard{
		Kind:   "EOA transaction",
		Prompt: "Sign and broadcast (≈ 0.0017 ETH)?",
	}
	if !ConfirmSigningCard(rec, card) {
		t.Fatal("scripted y should confirm")
	}
	if !rec.HasMessage("[12/87] EOA transaction") {
		t.Fatalf("section must carry [i/n]: %v", rec.Entries())
	}
	if !rec.HasMessage("[12/87] Sign and broadcast (≈ 0.0017 ETH)?") {
		t.Fatalf("prompt must carry [i/n]: %v", rec.Entries())
	}

	ClearBatchItem()
	rec = ui.NewRecordingUI()
	ShowSigningCard(rec, &SigningCard{Kind: "EOA transaction"})
	if rec.HasMessage("[12/87] EOA transaction") {
		t.Fatalf("cleared mark must not leak: %v", rec.Entries())
	}
	if !rec.HasMessage("EOA transaction") {
		t.Fatalf("single-tx card title lost: %v", rec.Entries())
	}
}
