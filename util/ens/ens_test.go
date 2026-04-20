package ens

import (
	"encoding/hex"
	"testing"
)

// TestNamehashEIP137Vectors pins our Namehash implementation to the
// worked examples in EIP-137. If these ever diverge, downstream
// resolutions will silently hit the wrong storage slot, so this test is
// the cheapest way to catch a regression early.
func TestNamehashEIP137Vectors(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{
			name: "",
			want: "0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			name: "eth",
			want: "93cdeb708b7545dc668eb9280176169d1c33cfd8ed6f04690a0bcc88a93fc4ae",
		},
		{
			name: "foo.eth",
			want: "de9b09fd7c5f901e23a3f19fecc54828e9c848539801e86591bd9801b019f84f",
		},
	}
	for _, tc := range cases {
		got := Namehash(tc.name)
		gotHex := hex.EncodeToString(got[:])
		if gotHex != tc.want {
			t.Errorf("Namehash(%q) = %s, want %s", tc.name, gotHex, tc.want)
		}
	}
}

// TestNamehashCaseInsensitive guards the documented contract that
// Namehash lowercases the input, so users who type Foo.Eth get the
// same node as foo.eth. Without this, cache hits would depend on
// capitalisation.
func TestNamehashCaseInsensitive(t *testing.T) {
	a := Namehash("Foo.ETH")
	b := Namehash("foo.eth")
	if a != b {
		t.Fatalf("expected case-insensitive namehash, got %x vs %x", a, b)
	}
}

func TestIsLikelyENSName(t *testing.T) {
	pos := []string{
		"alice.eth",
		"ALICE.eth",
		"  alice.eth  ",
		"foo.bar.eth",
		"a-b_c.eth",
		"123.eth",
	}
	for _, s := range pos {
		if !IsLikelyENSName(s) {
			t.Errorf("expected %q to be recognised as ENS name", s)
		}
	}
	neg := []string{
		"",
		"eth",
		".eth",
		"alice",
		"alice.com",
		"0x1234567890abcdef1234567890abcdef12345678",
		"Maker USDC - 6",
		"alice.eth/",
		"alice .eth",
		"alice..eth",
	}
	for _, s := range neg {
		if IsLikelyENSName(s) {
			t.Errorf("expected %q to NOT be recognised as ENS name", s)
		}
	}
}
