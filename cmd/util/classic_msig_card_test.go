package util

import (
	"math/big"
	"strings"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util/addrbook"
)

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

func TestDecodeClassicCalldataFallsBackToWETHWithdraw(t *testing.T) {
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
	weth := "0x0Bd7D308f8E1639FAb988df18A8011f41EAcAD73"
	data := ethcommon.FromHex("0x2e1a7d4d00000000000000000000000000000000000000000000000049f167f874d62fc2")

	analyzer := txanalyzer.NewGenericAnalyzerWithContext(
		txanalyzer.NewAnalysisContextWithResolver(nil, network, addrbook.Map{}),
	)
	fc := decodeClassicCalldata(weth, big.NewInt(0), data, network, stubResolver{}, analyzer)
	if fc == nil {
		t.Fatal("expected analyzer fallback, got nil call")
	}
	if fc.Method != "withdraw" {
		t.Fatalf("method = %q, want withdraw, err %q", fc.Method, fc.Error)
	}

	warns := SigningWarnings(WarningInput{
		To:      jarviscommon.Address{Address: weth, Desc: "RobinHood's WETH"},
		HasData: true,
		Call:    fc,
	})
	for _, w := range warns {
		if strings.Contains(w, "could not be decoded") {
			t.Fatalf("WETH fallback must not warn about a missing ABI: %q", w)
		}
	}
}
