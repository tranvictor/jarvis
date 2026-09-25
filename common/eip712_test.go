package common

import (
	"encoding/hex"
	"testing"
)

func TestEIP712DigestKnownVector(t *testing.T) {
	var domain, message [32]byte
	copy(domain[:], bytesFromHex(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	copy(message[:], bytesFromHex(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))

	got := EIP712Digest(domain, message)
	// keccak256(0x19 || 0x01 || domain || message)
	want := bytesFromHex(t, "5c8c173cae192c85f364b41c1120b53d71fc0dfb4036a805c48d7d5bb8490861")
	if hex.EncodeToString(got[:]) != hex.EncodeToString(want) {
		t.Fatalf("got %x, want %x", got, want)
	}
}

func bytesFromHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
