package walletconnect

import (
	"strings"
	"testing"
)

func TestParseURI_V2_Happy(t *testing.T) {
	raw := "wc:c9e6d3a63c0f4e8fae8b74e22b4a27c1f9aaa39d5f2d4c8b7eab2b3a11223344@2" +
		"?relay-protocol=irn&symKey=0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	u, err := ParseURI(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if u.Version != 2 {
		t.Errorf("version = %d want 2", u.Version)
	}
	if u.Topic != "c9e6d3a63c0f4e8fae8b74e22b4a27c1f9aaa39d5f2d4c8b7eab2b3a11223344" {
		t.Errorf("topic = %q", u.Topic)
	}
	if u.RelayProtocol != "irn" {
		t.Errorf("relay = %q", u.RelayProtocol)
	}
	if len(u.SymKey) != 32 {
		t.Errorf("symkey len = %d", len(u.SymKey))
	}
}

func TestParseURI_V2_MissingRelayProtocolDefaultsToIrn(t *testing.T) {
	raw := "wc:" + strings.Repeat("a", 64) + "@2" +
		"?symKey=" + strings.Repeat("ab", 32)
	u, err := ParseURI(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if u.RelayProtocol != "irn" {
		t.Errorf("relay = %q want irn", u.RelayProtocol)
	}
}

func TestParseURI_RejectsV1(t *testing.T) {
	raw := "wc:" + strings.Repeat("a", 64) + "@1?bridge=https://x&key=" + strings.Repeat("ab", 32)
	_, err := ParseURI(raw)
	if err == nil {
		t.Fatal("expected v1 rejection")
	}
	if !strings.Contains(err.Error(), "v1") {
		t.Errorf("err = %v, want it to mention v1", err)
	}
}

func TestParseURI_RejectsNonWC(t *testing.T) {
	_, err := ParseURI("https://example.com")
	if err == nil {
		t.Fatal("expected error for non-wc scheme")
	}
}

func TestParseURI_RejectsBadTopicLength(t *testing.T) {
	raw := "wc:deadbeef@2?symKey=" + strings.Repeat("ab", 32)
	_, err := ParseURI(raw)
	if err == nil {
		t.Fatal("expected error for short topic")
	}
}

func TestParseURI_RejectsMissingSymKey(t *testing.T) {
	raw := "wc:" + strings.Repeat("a", 64) + "@2?relay-protocol=irn"
	_, err := ParseURI(raw)
	if err == nil || !strings.Contains(err.Error(), "symKey") {
		t.Fatalf("expected symKey error, got %v", err)
	}
}

func TestParseURI_RejectsBadSymKeyLength(t *testing.T) {
	raw := "wc:" + strings.Repeat("a", 64) + "@2?symKey=abcd"
	_, err := ParseURI(raw)
	if err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("expected 32-byte symKey error, got %v", err)
	}
}
