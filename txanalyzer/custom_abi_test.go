package txanalyzer

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
)

func abiFromJSON(t *testing.T, json string) *abi.ABI {
	t.Helper()
	a, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		t.Fatalf("parse abi: %s", err)
	}
	return &a
}

const (
	setVTABI = `[{"type":"function","name":"setVT","inputs":[{"name":"newVT","type":"address"}],"outputs":[],"stateMutability":"nonpayable"}]`
	fullABI  = `[{"type":"function","name":"setVT","inputs":[{"name":"newVT","type":"address"}],"outputs":[],"stateMutability":"nonpayable"},
	             {"type":"function","name":"setOperator","inputs":[{"name":"operator","type":"address"},{"name":"allowed","type":"bool"}],"outputs":[],"stateMutability":"nonpayable"}]`
)

// TestPartialCustomABIFallsBackToLookup covers a custom ABI that describes the
// destination only partially — one --abi method, or one synthesised method per
// Safe batch entry. The selector it doesn't cover must still be resolved
// through the ABI database instead of rendering as an undecoded call.
func TestPartialCustomABIFallsBackToLookup(t *testing.T) {
	target := "0x1bD5af8e731D0969E8eBf7ea87f06d9Dc096d155"
	full := abiFromJSON(t, fullABI)
	data, err := full.Pack(
		"setOperator",
		ethcommon.HexToAddress("0x990bC4fE8B3f75Fe02F58b939a5fc4F76C725fc7"),
		true,
	)
	if err != nil {
		t.Fatalf("pack: %s", err)
	}

	lookup := func(address string, network jarvisnetworks.Network) (*abi.ABI, error) {
		return full, nil
	}
	customABIs := map[string]*abi.ABI{
		strings.ToLower(target): abiFromJSON(t, setVTABI),
	}

	fc := pureAnalyzer().AnalyzeFunctionCallRecursively(
		lookup, big.NewInt(0), target, data, customABIs,
	)
	if fc.Error != "" {
		t.Fatalf("unexpected error: %s", fc.Error)
	}
	if fc.Method != "setOperator" {
		t.Errorf("method = %q, want setOperator", fc.Method)
	}
}

// TestPartialCustomABIKeepsErrorWhenLookupFails documents the fail-closed half:
// with nothing better available the operator still gets the raw selector and
// the reason it couldn't be decoded.
func TestPartialCustomABIKeepsErrorWhenLookupFails(t *testing.T) {
	target := "0x1bD5af8e731D0969E8eBf7ea87f06d9Dc096d155"
	full := abiFromJSON(t, fullABI)
	data, err := full.Pack(
		"setOperator",
		ethcommon.HexToAddress("0x990bC4fE8B3f75Fe02F58b939a5fc4F76C725fc7"),
		true,
	)
	if err != nil {
		t.Fatalf("pack: %s", err)
	}

	customABIs := map[string]*abi.ABI{
		strings.ToLower(target): abiFromJSON(t, setVTABI),
	}
	fc := pureAnalyzer().AnalyzeFunctionCallRecursively(
		noABIFound, big.NewInt(0), target, data, customABIs,
	)
	if !strings.Contains(fc.Error, "no method with id") {
		t.Errorf("error = %q, want it to name the unknown selector", fc.Error)
	}
	if fc.Data == nil || len(fc.Data) != len(data) {
		t.Errorf("raw calldata not preserved for the undecodable call")
	}
}

func TestAnalyzeFallsBackToERC20WhenExplorerHasNoABI(t *testing.T) {
	target := "0x0Bd7D308f8E1639FAb988df18A8011f41EAcAD73"
	spender := ethcommon.HexToAddress("0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D")
	data, err := jarviscommon.GetERC20ABI().Pack("approve", spender, big.NewInt(1))
	if err != nil {
		t.Fatalf("pack: %s", err)
	}

	fc := pureAnalyzer().AnalyzeFunctionCallRecursively(
		noABIFound, big.NewInt(0), target, data, nil,
	)
	if fc.Error != "" {
		t.Fatalf("unexpected error: %s", fc.Error)
	}
	if fc.Method != "approve" {
		t.Fatalf("method = %q, want approve via ERC-20 fallback", fc.Method)
	}
}

func TestAnalyzeMethodlessCustomABIFallsBackToERC20(t *testing.T) {
	target := "0x0Bd7D308f8E1639FAb988df18A8011f41EAcAD73"
	spender := ethcommon.HexToAddress("0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D")
	data, err := jarviscommon.GetERC20ABI().Pack("approve", spender, big.NewInt(1))
	if err != nil {
		t.Fatalf("pack: %s", err)
	}
	proxy := abiFromJSON(t, `[{"anonymous":false,"inputs":[{"indexed":true,"name":"implementation","type":"address"}],"name":"Upgraded","type":"event"}]`)
	fc := pureAnalyzer().AnalyzeFunctionCallRecursively(
		noABIFound, big.NewInt(0), target, data,
		map[string]*abi.ABI{strings.ToLower(target): proxy},
	)
	if fc.Error != "" {
		t.Fatalf("unexpected error: %s", fc.Error)
	}
	if fc.Method != "approve" {
		t.Fatalf("method = %q, want approve via ERC-20 fallback", fc.Method)
	}
}

func TestAnalyzeFallsBackToGnosisMsigABIWhenExplorerHasNoABI(t *testing.T) {
	msig := "0x48B8419B2Bc0fB63Ee96e3a370e30B200cC2e672"
	data, err := util.GetGnosisMsigABI().Pack("confirmTransaction", big.NewInt(6))
	if err != nil {
		t.Fatalf("pack: %s", err)
	}
	fc := pureAnalyzer().AnalyzeFunctionCallRecursively(
		noABIFound, big.NewInt(0), msig, data, nil,
	)
	if fc.Error != "" {
		t.Fatalf("unexpected error: %s", fc.Error)
	}
	if fc.Method != "confirmTransaction" {
		t.Fatalf("method = %q, want confirmTransaction via Classic ABI fallback", fc.Method)
	}
	if len(fc.Params) != 1 || fc.Params[0].Values[0].Raw != "6" {
		t.Fatalf("transactionId not decoded: %+v", fc.Params)
	}
}

func TestAnalyzeFallsBackToGnosisMsigABIWhenProxyABIHasNoMethods(t *testing.T) {
	msig := "0x48B8419B2Bc0fB63Ee96e3a370e30B200cC2e672"
	data, err := util.GetGnosisMsigABI().Pack("confirmTransaction", big.NewInt(6))
	if err != nil {
		t.Fatalf("pack: %s", err)
	}
	proxy := abiFromJSON(t, `[{"inputs":[{"name":"_singleton","type":"address"}],"stateMutability":"nonpayable","type":"constructor"},{"stateMutability":"payable","type":"fallback"}]`)
	fc := pureAnalyzer().AnalyzeFunctionCallRecursively(
		noABIFound, big.NewInt(0), msig, data,
		map[string]*abi.ABI{strings.ToLower(msig): proxy},
	)
	if fc.Method != "confirmTransaction" {
		t.Fatalf("method = %q, want confirmTransaction, err %q", fc.Method, fc.Error)
	}
}
