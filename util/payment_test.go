package util

import (
	"math/big"
	"testing"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
)

func TestNativePaymentHeadline(t *testing.T) {
	to := jarviscommon.Address{Address: "0x9642b23Ed1E01Df1092B92641051881a322F5D4E", Desc: "Alice"}
	p := nativePayment(to, big.NewInt(1_500_000_000_000_000_000), networks.EthereumMainnet)
	if p == nil || p.Amount != "1.5 ETH" {
		t.Fatalf("amount = %+v", p)
	}
	if p.To.Text != jarviscommon.PlainAddress(to) {
		t.Fatalf("to = %q", p.To.Text)
	}
}

func TestTokenPaymentTransfer(t *testing.T) {
	to := jarviscommon.Address{Address: "0x9642b23Ed1E01Df1092B92641051881a322F5D4E", Desc: "Alice"}
	amount := jarviscommon.Value{
		Raw:   "1000000000",
		Kind:  jarviscommon.DisplayToken,
		Token: &jarviscommon.TokenHint{Decimal: 6, Symbol: "USDC"},
	}
	fc := &jarviscommon.FunctionCall{
		Destination: jarviscommon.Address{Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Desc: "USDC token"},
		Method:      "transfer",
		Params: []jarviscommon.ParamResult{
			{Name: "_to", Type: "address", Values: []jarviscommon.Value{{Raw: to.Address, Kind: jarviscommon.DisplayAddress, Address: &to}}},
			{Name: "_value", Type: "uint256", Values: []jarviscommon.Value{amount}},
		},
	}
	p := tokenPayment(fc)
	if p == nil || p.Amount != "1,000 USDC" {
		t.Fatalf("amount = %+v", p)
	}
	if p.To.Text != jarviscommon.PlainAddress(to) {
		t.Fatalf("to = %q", p.To.Text)
	}

	rec := ui.NewRecordingUI()
	PrintFunctionCall(rec, NewFunctionCallDisplay(fc, networks.EthereumMainnet))
	if !rec.HasMessage("Send  1,000 USDC  →  " + to.Address + " (Alice)") {
		t.Fatalf("missing send headline: %v", rec.Entries())
	}
}

func TestTokenPaymentTransferFrom(t *testing.T) {
	from := jarviscommon.Address{Address: "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D", Desc: "Router"}
	to := jarviscommon.Address{Address: "0x9642b23Ed1E01Df1092B92641051881a322F5D4E", Desc: "Alice"}
	fc := &jarviscommon.FunctionCall{
		Destination: jarviscommon.Address{Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Desc: "USDC", Decimal: 6},
		Method:      "transferFrom",
		Params: []jarviscommon.ParamResult{
			{Name: "from", Type: "address", Values: []jarviscommon.Value{{Kind: jarviscommon.DisplayAddress, Address: &from}}},
			{Name: "to", Type: "address", Values: []jarviscommon.Value{{Kind: jarviscommon.DisplayAddress, Address: &to}}},
			{Name: "amount", Type: "uint256", Values: []jarviscommon.Value{{Raw: "5000000", Kind: jarviscommon.DisplayInteger}}},
		},
	}
	p := tokenPayment(fc)
	if p == nil || p.Amount != "5 USDC" || p.From.Text == "" {
		t.Fatalf("%+v", p)
	}
}

func TestNativeInnerCallRendersAsSend(t *testing.T) {
	alice := jarviscommon.Address{Address: "0x9642b23Ed1E01Df1092B92641051881a322F5D4E", Desc: "Alice"}
	outer := &jarviscommon.FunctionCall{
		Destination: jarviscommon.Address{Address: "0x40A2aCCbd92BCA938b02010E17A5b8929b67764f", Desc: "MultiSendCallOnly"},
		Method:      "multiSend",
		DecodedFunctionCalls: []*jarviscommon.FunctionCall{{
			Destination: alice,
			Value:       big.NewInt(1_500_000_000_000_000_000),
		}},
	}
	rec := ui.NewRecordingUI()
	PrintFunctionCall(rec, NewFunctionCallDisplay(outer, networks.EthereumMainnet))
	if !rec.HasMessage("Send  1.5 ETH  →  " + alice.Address + " (Alice)") {
		t.Fatalf("inner ETH send: %v", rec.Entries())
	}
	if rec.HasMessage("<undecoded>") {
		t.Fatalf("native send must not look undecoded: %v", rec.Entries())
	}
}
