package txanalyzer

import (
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util/addrbook"
)

// noABIFound stands in for the block-explorer lookup: MultiSendCallOnly and the
// batched targets are frequently unverified, which is exactly the case the
// analyzer has to handle without hiding anything from the operator.
func noABIFound(address string, network jarvisnetworks.Network) (*abi.ABI, error) {
	return nil, fmt.Errorf("no abi for %s", address)
}

func pureAnalyzer() *TxAnalyzer {
	ctx := NewAnalysisContextWithResolver(nil, jarvisnetworks.BSCMainnet, addrbook.Map{})
	return NewGenericAnalyzerWithContext(ctx)
}

// TestAnalyzeMultiSendBatchExpandsInnerCalls is the safety property that
// motivated MultiSend decoding: an owner reviewing a batched proposal must see
// every call it makes, not one opaque bytes argument.
func TestAnalyzeMultiSendBatchExpandsInnerCalls(t *testing.T) {
	erc20 := jarviscommon.GetERC20ABI()
	tokenA := ethcommon.HexToAddress("0x55d398326f99059fF775485246999027B3197955")
	tokenB := ethcommon.HexToAddress("0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d")
	recipient := ethcommon.HexToAddress("0xBeEEE605DC6a531AeB4bc3C809Cf6Dd86674F001")
	spender := ethcommon.HexToAddress("0x1af18F06F97679B16A8F553326ab2857e6cFd920")

	transferData, err := erc20.Pack("transfer", recipient, big.NewInt(1234))
	if err != nil {
		t.Fatalf("pack transfer: %s", err)
	}
	approveData, err := erc20.Pack("approve", spender, big.NewInt(5678))
	if err != nil {
		t.Fatalf("pack approve: %s", err)
	}

	packed, err := jarviscommon.PackMultiSend([]jarviscommon.MultiSendCall{
		{Operation: 0, To: tokenA, Value: big.NewInt(0), Data: transferData},
		{Operation: 0, To: tokenB, Value: big.NewInt(0), Data: approveData},
	})
	if err != nil {
		t.Fatalf("pack multiSend: %s", err)
	}

	multiSendAddr := "0x40A2aCCbd92BCA938b02010E17A5b8929b49130D"
	fc := pureAnalyzer().AnalyzeFunctionCallRecursively(
		noABIFound, big.NewInt(0), multiSendAddr, packed, nil,
	)

	// The multiSend method itself must decode even though the lookup failed,
	// via the built-in fallback ABI.
	if fc.Method != jarviscommon.MultiSendMethodName {
		t.Fatalf("method = %q, want %q", fc.Method, jarviscommon.MultiSendMethodName)
	}
	if fc.Error != "" {
		t.Errorf("unexpected error on the outer call: %s", fc.Error)
	}
	if len(fc.DecodedFunctionCalls) != 2 {
		t.Fatalf("got %d inner calls, want 2", len(fc.DecodedFunctionCalls))
	}

	inner := fc.DecodedFunctionCalls
	if inner[0].Method != "transfer" {
		t.Errorf("inner call 0 method = %q, want transfer", inner[0].Method)
	}
	if inner[1].Method != "approve" {
		t.Errorf("inner call 1 method = %q, want approve", inner[1].Method)
	}
	for i, want := range []string{tokenA.Hex(), tokenB.Hex()} {
		if got := inner[i].Destination.Address; got != want {
			t.Errorf("inner call %d destination = %s, want %s", i, got, want)
		}
	}
}

// TestAnalyzeMultiSendUsesCustomABIs covers the tx-builder path, where jarvis
// supplies the ABIs it just encoded with instead of relying on the explorer.
func TestAnalyzeMultiSendUsesCustomABIs(t *testing.T) {
	target := ethcommon.HexToAddress("0x1d9937e170Fc2174408581265bA0B87afDA4947F")
	custom, err := abi.JSON(strings.NewReader(`[{"type":"function","name":"whitelist","inputs":[
		{"name":"settlementContract","type":"address"},{"name":"signingEoa","type":"address"}],
		"outputs":[],"stateMutability":"nonpayable"}]`))
	if err != nil {
		t.Fatalf("parse custom abi: %s", err)
	}
	inner, err := custom.Pack("whitelist",
		ethcommon.HexToAddress("0x1af18F06F97679B16A8F553326ab2857e6cFd920"),
		ethcommon.HexToAddress("0xBeEEE605DC6a531AeB4bc3C809Cf6Dd86674F001"),
	)
	if err != nil {
		t.Fatalf("pack whitelist: %s", err)
	}

	packed, err := jarviscommon.PackMultiSend([]jarviscommon.MultiSendCall{
		{Operation: 0, To: target, Value: big.NewInt(0), Data: inner},
		{Operation: 0, To: target, Value: big.NewInt(0), Data: inner},
	})
	if err != nil {
		t.Fatalf("pack multiSend: %s", err)
	}

	fc := pureAnalyzer().AnalyzeFunctionCallRecursively(
		noABIFound, big.NewInt(0), "0x40A2aCCbd92BCA938b02010E17A5b8929b49130D", packed,
		map[string]*abi.ABI{"0x1d9937e170fc2174408581265ba0b87afda4947f": &custom},
	)

	if len(fc.DecodedFunctionCalls) != 2 {
		t.Fatalf("got %d inner calls, want 2", len(fc.DecodedFunctionCalls))
	}
	for i, c := range fc.DecodedFunctionCalls {
		if c.Method != "whitelist" {
			t.Errorf("inner call %d method = %q, want whitelist", i, c.Method)
		}
		if len(c.Params) != 2 {
			t.Errorf("inner call %d has %d params, want 2", i, len(c.Params))
		}
	}
}

// TestAnalyzeMultiSendFlagsDelegateCallEntry checks that a batch entry asking
// for a DELEGATECALL is called out rather than blending in with normal calls.
func TestAnalyzeMultiSendFlagsDelegateCallEntry(t *testing.T) {
	erc20 := jarviscommon.GetERC20ABI()
	data, err := erc20.Pack("approve",
		ethcommon.HexToAddress("0x1af18F06F97679B16A8F553326ab2857e6cFd920"), big.NewInt(1))
	if err != nil {
		t.Fatalf("pack: %s", err)
	}
	packed, err := jarviscommon.PackMultiSend([]jarviscommon.MultiSendCall{{
		Operation: 1,
		To:        ethcommon.HexToAddress("0x55d398326f99059fF775485246999027B3197955"),
		Value:     big.NewInt(0),
		Data:      data,
	}})
	if err != nil {
		t.Fatalf("pack multiSend: %s", err)
	}

	fc := pureAnalyzer().AnalyzeFunctionCallRecursively(
		noABIFound, big.NewInt(0), "0x40A2aCCbd92BCA938b02010E17A5b8929b49130D", packed, nil,
	)
	if len(fc.DecodedFunctionCalls) != 1 {
		t.Fatalf("got %d inner calls, want 1", len(fc.DecodedFunctionCalls))
	}
	if fc.DecodedFunctionCalls[0].Error == "" {
		t.Error("a DELEGATECALL batch entry should be flagged")
	}
}

// TestAnalyzeMultiSendMalformedPayloadSurfacesError makes sure a batch jarvis
// can't read never renders as a batch with nothing in it.
func TestAnalyzeMultiSendMalformedPayloadSurfacesError(t *testing.T) {
	packed, err := jarviscommon.GetMultiSendABI().Pack(
		jarviscommon.MultiSendMethodName, []byte{0x00, 0x01, 0x02},
	)
	if err != nil {
		t.Fatalf("pack: %s", err)
	}

	fc := pureAnalyzer().AnalyzeFunctionCallRecursively(
		noABIFound, big.NewInt(0), "0x40A2aCCbd92BCA938b02010E17A5b8929b49130D", packed, nil,
	)
	if len(fc.DecodedFunctionCalls) != 1 {
		t.Fatalf("got %d children, want 1 error entry", len(fc.DecodedFunctionCalls))
	}
	if fc.DecodedFunctionCalls[0].Error == "" {
		t.Error("expected the decode failure to be surfaced as a child error")
	}
}
