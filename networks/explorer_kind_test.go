package networks

import (
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/util/explorers"
)

func TestBuiltinExplorerKinds(t *testing.T) {
	if EthereumMainnet.(*GenericEtherscanNetwork).Kind != explorers.KindEtherscan {
		t.Fatalf("mainnet kind = %q", EthereumMainnet.(*GenericEtherscanNetwork).Kind)
	}
	if Avalanche.(*GenericEtherscanNetwork).Kind != explorers.KindRoutescan {
		t.Fatalf("avalanche kind = %q", Avalanche.(*GenericEtherscanNetwork).Kind)
	}
	if BitfiTestnet.Kind != explorers.KindBlockscout {
		t.Fatalf("bitfi kind = %q", BitfiTestnet.Kind)
	}
}

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

func TestExplorerKindOmittedFromBuiltinJSON(t *testing.T) {
	raw, err := EthereumMainnet.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"explorer_kind"`) {
		t.Fatalf("unset explorer_kind should omit: %s", raw)
	}
	raw, err = Avalanche.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"explorer_kind":"routescan"`) {
		t.Fatalf("avalanche should persist explorer_kind routescan: %s", raw)
	}
}
