package common

import "testing"

func TestShortAddress(t *testing.T) {
	cases := map[string]string{
		"0x9642b23Ed1E01Df1092B92641051881a322F5D4E": "0x9642…5D4E",
		"0xabc":           "0xabc",
		"":                "",
		"0x1234567890abc": "0x1234…0abc",
	}
	for in, want := range cases {
		if got := ShortAddress(in); got != want {
			t.Errorf("ShortAddress(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNameFirst(t *testing.T) {
	known := Address{Address: "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D", Desc: "Uniswap V2 Router"}
	unknown := Address{Address: "0x9642b23Ed1E01Df1092B92641051881a322F5D4E", Desc: "unknown"}
	blank := Address{Address: "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"}

	cases := []struct {
		addr Address
		full bool
		want string
	}{
		{known, false, "Uniswap V2 Router (0x7a25…488D)"},
		{known, true, "Uniswap V2 Router (0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D)"},
		{unknown, false, "0x9642…5D4E (unknown)"},
		{blank, true, "0x9642b23Ed1E01Df1092B92641051881a322F5D4E (unknown)"},
		{Address{}, false, ""},
	}
	for _, c := range cases {
		if got := NameFirst(c.addr, c.full); got != c.want {
			t.Errorf("NameFirst(%v, %v) = %q, want %q", c.addr, c.full, got, c.want)
		}
	}
}

func TestIsKnownAddress(t *testing.T) {
	if IsKnownAddress(Address{Desc: "unknown"}) || IsKnownAddress(Address{}) {
		t.Fatal("unknown/blank descriptions must not count as known")
	}
	if !IsKnownAddress(Address{Desc: "USDC"}) {
		t.Fatal("named address must count as known")
	}
}
