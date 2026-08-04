package networks

import (
	"strings"
	"testing"
)

func TestGetSafeTxServiceURLFromJSON(t *testing.T) {
	n, err := NewNetworkFromJSON([]byte(`{
		"name": "custom",
		"chain_id": 4663,
		"native_token_symbol": "ETH",
		"native_token_decimal": 18,
		"safe_tx_service_url": "https://safe-tx.example.com/  "
	}`))
	if err != nil {
		t.Fatalf("NewNetworkFromJSON failed: %s", err)
	}
	// Trailing slash and whitespace are normalised away so callers can
	// concatenate paths onto the result unconditionally.
	if got := n.GetSafeTxServiceURL(); got != "https://safe-tx.example.com" {
		t.Errorf("GetSafeTxServiceURL() = %q, want the trimmed URL", got)
	}
}

func TestGetSafeTxServiceURLDefaultsEmpty(t *testing.T) {
	n, err := NewNetworkFromJSON([]byte(`{"name": "custom", "chain_id": 4663}`))
	if err != nil {
		t.Fatalf("NewNetworkFromJSON failed: %s", err)
	}
	if got := n.GetSafeTxServiceURL(); got != "" {
		t.Errorf("GetSafeTxServiceURL() = %q, want empty when the field is absent", got)
	}

	// The field is optional, so it must not appear in the marshalled
	// config of a network that never set it.
	raw, err := n.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %s", err)
	}
	if got := string(raw); strings.Contains(got, "safe_tx_service_url") {
		t.Errorf("marshalled config %s should omit an unset safe_tx_service_url", got)
	}
}

func TestGenericOptimismSafeTxServiceURL(t *testing.T) {
	n := NewGenericOptimismNetwork(GenericOptimismNetworkConfig{
		Name:             "custom-op",
		ChainID:          4664,
		SafeTxServiceURL: "https://safe-tx-op.example.com/",
	})
	if got := n.GetSafeTxServiceURL(); got != "https://safe-tx-op.example.com" {
		t.Errorf("GetSafeTxServiceURL() = %q, want the trimmed URL", got)
	}
}
