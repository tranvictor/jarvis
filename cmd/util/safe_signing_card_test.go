package util

import (
	"math/big"
	"strings"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util/addrbook"
)

func TestDecodeSafeCalldataFallsBackToERC20WhenExplorerHasNoABI(t *testing.T) {
	prevForce := config.ForceERC20ABI
	prevCustom := config.CustomABI
	config.ForceERC20ABI = false
	config.CustomABI = ""
	t.Cleanup(func() {
		config.ForceERC20ABI = prevForce
		config.CustomABI = prevCustom
	})

	network, err := jarvisnetworks.GetNetwork("mainnet")
	if err != nil {
		t.Fatal(err)
	}
	token := ethcommon.HexToAddress("0xa11ce00000000000000000000000000000000001")
	spender := ethcommon.HexToAddress("0xa11ce00000000000000000000000000000000002")
	data, err := jarviscommon.GetERC20ABI().Pack("approve", spender, big.NewInt(1))
	if err != nil {
		t.Fatal(err)
	}

	analyzer := txanalyzer.NewGenericAnalyzerWithContext(
		txanalyzer.NewAnalysisContextWithResolver(nil, network, addrbook.Map{}),
	)
	fc := decodeSafeCalldata(
		&safe.SafeTx{To: token, Value: big.NewInt(0), Data: data},
		network,
		stubResolver{},
		analyzer,
		nil,
	)
	if fc == nil {
		t.Fatal("expected analyzer fallback, got nil call")
	}
	if fc.Method != "approve" {
		t.Fatalf("method = %q, want approve", fc.Method)
	}

	warns := SigningWarnings(WarningInput{
		To:      jarviscommon.Address{Address: token.Hex(), Desc: "Token"},
		HasData: true,
		Call:    fc,
	})
	for _, w := range warns {
		if strings.Contains(w, "could not be decoded") {
			t.Fatalf("ERC-20 fallback must not warn about a missing ABI: %q", w)
		}
	}
}
