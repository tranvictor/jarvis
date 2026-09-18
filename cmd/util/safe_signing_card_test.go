package util

import (
	"math/big"
	"strings"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/accounts/types"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/ui"
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

func TestSafeSignerLineNamesLocalWallet(t *testing.T) {
	const owner = "0xa3759774994F5012E5d725dCC1B96750945C793f"
	accounts.SetWalletsForTest(map[string]types.AccDesc{
		owner: {Address: owner, Kind: "ledger", Desc: "work ledger"},
	})
	t.Cleanup(func() { accounts.SetWalletsForTest(nil) })

	network, err := jarvisnetworks.GetNetwork("mainnet")
	if err != nil {
		t.Fatal(err)
	}
	ownerAddr := ethcommon.HexToAddress(owner)
	line := safeSignerLine(safe.OwnerSig{Owner: ownerAddr}, network)
	if !strings.Contains(line.Text, ownerAddr.Hex()) {
		t.Fatalf("must keep the full address: %q", line.Text)
	}
	if !strings.Contains(line.Text, "work ledger - your wallet") && !strings.Contains(line.Text, "your wallet") {
		t.Fatalf("local wallet must be named on the Signed-by line: %q", line.Text)
	}

	rec := ui.NewRecordingUI()
	ShowSigningCard(rec, &SigningCard{
		Kind: "Safe approval",
		Safe: &SafeCardFields{
			Signatures: []ui.StyledText{line},
			Threshold:  2,
		},
	})
	if !rec.HasMessage("Signed by (1 of 2 required)") {
		t.Fatalf("heading missing: %v", rec.Entries())
	}
	found := false
	for _, e := range rec.Entries() {
		if strings.Contains(e.Value, "work ledger") && strings.Contains(e.Value, "your wallet") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("signing card must show the wallet name: %v", rec.Entries())
	}
}
