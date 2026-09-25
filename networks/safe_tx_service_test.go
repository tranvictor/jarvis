package networks

import "testing"

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
