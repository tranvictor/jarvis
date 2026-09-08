package util

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util/addrbook"
)

const withdrawABIJSON = `[{"type":"function","name":"withdraw","inputs":[{"name":"wad","type":"uint256"}],"outputs":[],"stateMutability":"nonpayable"}]`

type followedABIResolver struct {
	stubResolver
	a *abi.ABI
}

func (r followedABIResolver) GetABI(string, jarvisnetworks.Network) (*abi.ABI, error) {
	return r.a, nil
}
func (r followedABIResolver) ConfigToABI(string, bool, string, jarvisnetworks.Network) (*abi.ABI, error) {
	return r.a, nil
}

func TestClassicSummaryStatus(t *testing.T) {
	cases := []struct {
		confirmed int
		required  int64
		executed  bool
		want      string
	}{
		{0, 2, true, "executed"},
		{2, 2, false, "ready to execute"},
		{3, 2, false, "ready to execute"},
		{1, 2, false, "pending"},
		{0, 0, false, "pending"},
	}
	for _, c := range cases {
		got := ClassicSummaryStatus(c.confirmed, c.required, c.executed)
		if got != c.want {
			t.Errorf("ClassicSummaryStatus(%d, %d, %v) = %q, want %q",
				c.confirmed, c.required, c.executed, got, c.want)
		}
	}
}

func TestDecodeClassicCalldataUsesImplementationABI(t *testing.T) {
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
	target := "0xa11ce00000000000000000000000000000000001"
	data := ethcommon.FromHex("0x2e1a7d4d0000000000000000000000000000000000000000000000000000000000000001")
	implABI, err := abi.JSON(strings.NewReader(withdrawABIJSON))
	if err != nil {
		t.Fatal(err)
	}

	analyzer := txanalyzer.NewGenericAnalyzerWithContext(
		txanalyzer.NewAnalysisContextWithResolver(nil, network, addrbook.Map{}),
	)
	fc := decodeSigningCalldata(target, big.NewInt(0), data, network, followedABIResolver{a: &implABI}, analyzer, nil)
	if fc == nil {
		t.Fatal("expected analyzer fallback, got nil call")
	}
	if fc.Method != "withdraw" {
		t.Fatalf("method = %q, want withdraw, err %q", fc.Method, fc.Error)
	}

	warns := SigningWarnings(WarningInput{
		To:      jarviscommon.Address{Address: target, Desc: "Token"},
		HasData: true,
		Call:    fc,
	})
	for _, w := range warns {
		if strings.Contains(w, "could not be decoded") {
			t.Fatalf("followed implementation ABI must not warn about a missing ABI: %q", w)
		}
	}
}
