package common

import "testing"

func TestLooksLikeAddress(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"0xAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAa", true},
		{"0x0000000000000000000000000000000000000000", true},
		{" 0xAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAa ", true}, // trimmed
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},     // bare hex, no 0x prefix, still address-shaped
		{"0xAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaA", false},   // too short
		{"Some Label multisig", false},
		{"multisig", false},
		{"", false},
	}
	for _, c := range cases {
		if got := LooksLikeAddress(c.input); got != c.want {
			t.Errorf("LooksLikeAddress(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}
