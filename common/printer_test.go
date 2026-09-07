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
		{unknown, false, "0x9642…5D4E"},
		{blank, true, "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"},
		{Address{Address: "0x0000000000000000000000000000000000000000"}, false, "0x0000…0000 (zero address)"},
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

func TestGroupDigitsAndReadableNumber(t *testing.T) {
	cases := map[string]string{
		"1000000000":  "1,000,000,000",
		"1234567.891": "1,234,567.891",
		"-1234567":    "-1,234,567",
		"999":         "999",
		"0.5":         "0.5",
		"abc":         "abc",
	}
	for in, want := range cases {
		if got := GroupDigits(in); got != want {
			t.Errorf("GroupDigits(%q) = %q, want %q", in, got, want)
		}
	}
	if got := ReadableNumber("1000000000"); got != "1000000000 (1,000,000,000)" {
		t.Errorf("ReadableNumber = %q", got)
	}
	if got := ReadableNumber("1234"); got != "1234" {
		t.Errorf("short numbers stay bare, got %q", got)
	}
}

func TestCompactAmount(t *testing.T) {
	cases := map[string]string{
		"1000":                          "1,000",
		"1000.5":                        "1,000.5",
		"2552732552.244911963753134392": "2,552,732,552.2449",
		"14.633701431326966591":         "14.6337",
		"0.3121":                        "0.3121",
		"0.051932126031271887":          "0.05193",
		"0.000000123456":                "0.0000001234",
		"1.00001":                       "1",
		"0.0000000000000000001":         "0.0000000000000000001",
		"0":                             "0",
	}
	for in, want := range cases {
		if got := CompactAmount(in); got != want {
			t.Errorf("CompactAmount(%q) = %q, want %q", in, got, want)
		}
	}
}
