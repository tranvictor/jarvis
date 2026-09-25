package networks

import (
	"testing"

	"github.com/tranvictor/jarvis/util/explorers"
)

func TestNewNetworkFromJSONInfersExplorerKind(t *testing.T) {
	n, err := NewNetworkFromJSON([]byte(`{
		"name": "custom-bitfi",
		"chain_id": 891891,
		"block_explorer_api_url": "https://bitfi-ledger-testnet-explorer.alt.technology/api/v2"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	got := n.(*GenericEtherscanNetwork).Kind
	if got != explorers.KindBlockscout {
		t.Fatalf("inferred kind = %q, want blockscout", got)
	}

	n, err = NewNetworkFromJSON([]byte(`{
		"name": "custom-robinhood",
		"chain_id": 4663,
		"block_explorer_api_url": "https://robinscan.io/api",
		"explorer_kind": "robinscan"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	got = n.(*GenericEtherscanNetwork).Kind
	if got != explorers.KindRobinscan {
		t.Fatalf("explicit kind = %q, want robinscan", got)
	}
}
