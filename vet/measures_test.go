package vet

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

const (
	testUSDC   = "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"
	testRouter = "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"
	testMe     = "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"
)

func addr(hex, desc string) jarviscommon.Address {
	return jarviscommon.Address{Address: hex, Desc: desc}
}

func addrP(name, hex, desc string) jarviscommon.ParamResult {
	a := addr(hex, desc)
	return jarviscommon.ParamResult{Name: name, Type: "address", Values: []jarviscommon.Value{
		{Raw: hex, Kind: jarviscommon.DisplayAddress, Address: &a},
	}}
}

func uintP(name, raw string) jarviscommon.ParamResult {
	return jarviscommon.ParamResult{Name: name, Type: "uint256", Values: []jarviscommon.Value{
		{Raw: raw, Kind: jarviscommon.DisplayInteger},
	}}
}

func TestDelegateCallAlwaysOn(t *testing.T) {
	r := Analyze(context.Background(), Request{Mode: ModeAlways, DelegateCall: true, MultiSend: true})
	if len(r.Findings) != 1 || r.Findings[0].Code != CodeDelegateCall {
		t.Fatalf("got %+v", r.Findings)
	}
	if r.Findings[0].Risk != RiskDanger {
		t.Fatalf("risk %s", r.Findings[0].Risk)
	}
	r = Analyze(context.Background(), Request{Mode: ModeAlways, DelegateCall: true})
	if !strings.Contains(r.Findings[0].Text, "Safe's own context") {
		t.Fatalf("text %q", r.Findings[0].Text)
	}
}

func TestAdminUpgradeDrainMinOutRecipient(t *testing.T) {
	fc := &jarviscommon.FunctionCall{
		Destination: addr(testUSDC, "USDC"),
		Method:      "transferOwnership",
		Params:      []jarviscommon.ParamResult{addrP("newOwner", testRouter, "Router")},
		DecodedFunctionCalls: []*jarviscommon.FunctionCall{
			{
				Destination: addr(testUSDC, "USDC"),
				Method:      "upgradeTo",
				Params:      []jarviscommon.ParamResult{addrP("newImplementation", testMe, "me")},
			},
			{
				Destination: addr(testUSDC, "Vault"),
				Method:      "rescueTokens",
			},
			{
				Destination: addr(testRouter, "Router"),
				Method:      "swapExactTokensForTokens",
				Params:      []jarviscommon.ParamResult{uintP("amountOutMin", "0")},
			},
			{
				Destination: addr(testUSDC, "USDC"),
				Method:      "transfer",
				Params: []jarviscommon.ParamResult{
					addrP("to", testMe, "unknown"),
					uintP("amount", "1"),
				},
			},
		},
	}
	r := Analyze(context.Background(), Request{Mode: ModeFull, Call: fc, Book: []BookAddr{}})
	want := map[string]bool{CodeAdmin: false, CodeUpgrade: false, CodeDrain: false, CodeMinOutZero: false, CodeTokenRecipient: false}
	for _, f := range r.Findings {
		if _, ok := want[f.Code]; ok {
			want[f.Code] = true
		}
		if f.Code == CodeDrain && f.Risk != RiskDanger {
			t.Fatalf("drain must be danger/red, got %s", f.Risk)
		}
	}
	for code, found := range want {
		if !found {
			t.Errorf("missing %s in %+v", code, r.Findings)
		}
	}
}

func TestPoison2Plus2(t *testing.T) {
	// same first 2 and last 2 bytes as testMe, different middle
	poison := "0x9642aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa5D4E"
	r := Analyze(context.Background(), Request{
		Mode: ModeFull,
		To:   addr(poison, ""),
		Book: []BookAddr{{Hex: testMe, Label: "me"}},
	})
	if !hasFinding(r, CodePoison) {
		t.Fatalf("expected poison, got %+v", r.Findings)
	}
	if !strings.Contains(r.Findings[0].Text, "me") {
		t.Fatalf("card may show label: %q", r.Findings[0].Text)
	}
}

func TestUnverifiedAndCreate(t *testing.T) {
	r := Analyze(context.Background(), Request{Mode: ModeFull, Create: true})
	if !hasFinding(r, CodeCreate) {
		t.Fatalf("create: %+v", r.Findings)
	}
	r = Analyze(context.Background(), Request{
		Mode:  ModeFull,
		To:    addr(testUSDC, "USDC"),
		Chain: fakeLookup{src: Source{Address: testUSDC}},
	})
	if !hasFinding(r, CodeUnverified) {
		t.Fatalf("unverified: %+v", r.Findings)
	}
}

func TestGrokReconfirmAndSkipWhenUnverified(t *testing.T) {
	fc := &jarviscommon.FunctionCall{Destination: addr(testUSDC, "CANARY_UncleBob"), Method: "upgradeTo"}
	ai := &fakeAI{reply: ModelReply{
		Risk: "danger", Bullets: []string{"this upgrades the proxy"}, Reconfirms: []string{"upgrade"},
	}}
	r := Analyze(context.Background(), Request{
		Mode:  ModeFull,
		To:    addr(testUSDC, "CANARY_UncleBob"),
		Call:  fc,
		Chain: fakeLookup{src: Source{Address: testUSDC, Code: "contract C { function upgradeTo(address a) public {} }", Verified: true}},
		AI:    ai,
	})
	var upgrade Finding
	var grok int
	for _, f := range r.Findings {
		if f.Code == CodeUpgrade {
			upgrade = f
		}
		if f.Code == CodeGrok {
			grok++
		}
	}
	if !upgrade.GrokReconfirm {
		t.Fatalf("expected reconfirm on upgrade: %+v", r.Findings)
	}
	if grok == 0 {
		t.Fatal("expected grok bullets to be kept")
	}
	if ai.saw != "" && strings.Contains(ai.saw, "CANARY_UncleBob") {
		t.Fatalf("address-book name leaked to Grok: %s", ai.saw)
	}

	// unverified must not call Grok
	ai2 := &fakeAI{reply: ModelReply{Risk: "ok", Bullets: []string{"fine"}}}
	r = Analyze(context.Background(), Request{
		Mode:  ModeFull,
		To:    addr(testUSDC, "USDC"),
		Chain: fakeLookup{src: Source{Address: testUSDC}},
		AI:    ai2,
	})
	if ai2.called {
		t.Fatal("Grok must not run on unverified source")
	}
	if hasFinding(r, CodeGrok) {
		t.Fatalf("no grok findings on unverified: %+v", r.Findings)
	}
}

func TestTypedPermit(t *testing.T) {
	r := AnalyzeTypedData(context.Background(), TypedRequest{
		Mode:           ModeFull,
		NetworkChainID: 1,
		NetworkName:    "mainnet",
		PrimaryType:    "Permit",
		Verifying:      testUSDC,
		DomainChainID:  big.NewInt(137),
		Message: map[string]interface{}{
			"owner": testMe, "spender": testRouter, "value": maxUint256.String(),
		},
		Book:  []BookAddr{},
		Chain: fakeLookup{src: Source{Address: testUSDC, Verified: true, Code: "contract T { function permit() public {} }"}},
	})
	if !hasFinding(r, CodeTypedChainID) || !hasFinding(r, CodeTypedPermit) {
		t.Fatalf("typed permit: %+v", r.Findings)
	}
}

func hasFinding(r Report, code string) bool {
	for _, f := range r.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

type fakeLookup struct{ src Source }

func (f fakeLookup) Source(addr string) (Source, error) { return f.src, nil }
func (f fakeLookup) Implementation(addr string) (string, error) {
	return f.src.Implementation, nil
}

type fakeAI struct {
	reply  ModelReply
	saw    string
	called bool
}

func (f *fakeAI) Complete(ctx context.Context, payload []byte) (ModelReply, error) {
	f.called = true
	f.saw = string(payload)
	return f.reply, nil
}

func TestPoisonDoesNotMatchExactBookEntry(t *testing.T) {
	r := Analyze(context.Background(), Request{
		Mode: ModeFull,
		To:   addr(testMe, "me"),
		Book: []BookAddr{{Hex: testMe, Label: "me"}},
	})
	if hasFinding(r, CodePoison) {
		t.Fatalf("exact match is not poison: %+v", r.Findings)
	}
}

func TestHexRoundTrip(t *testing.T) {
	if common.HexToAddress(testMe).Hex() == "" {
		t.Fatal("sanity")
	}
}
