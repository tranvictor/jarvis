package util

import "testing"

func TestCompactAddressText(t *testing.T) {
	const a = "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"
	const b = "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"
	const hash = "0x3f9a1c2b4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e1c2"

	cases := map[string]string{
		a:           a[:6] + "…5D4E",
		a + " (me)": "me (0x9642…5D4E)",
		"0x0000000000000000000000000000000000000000 (Quang Le)": "0x0000…0000 (zero address)",
		a + " (USDC - 6)":                 "USDC (0x9642…5D4E)",
		a + " (unknown)":                  "0x9642…5D4E",
		"from " + a + " to " + b:          "from 0x9642…5D4E to 0x7a25…488D",
		a + "," + b:                       "0x9642…5D4E,0x7a25…488D",
		hash:                              hash,
		"1000000000 (1000 USDC)":          "1000000000 (1000 USDC)",
		"0x" + a[2:] + "deadbeef":         "0x" + a[2:] + "deadbeef",
		"[" + a + " (me), " + b + " (r)]": "[me (0x9642…5D4E), r (0x7a25…488D)]",
	}
	for in, want := range cases {
		if got := compactAddressText(in); got != want {
			t.Errorf("compactAddressText(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestCompactHex(t *testing.T) {
	long := "0x" + repeatHex("ab", 40)
	if got := compactHex(long); got != "0xabab…abab (40 bytes)" {
		t.Fatalf("got %q", got)
	}
	hash := "0x" + repeatHex("cd", 32)
	if got := compactHex(hash); got != hash {
		t.Fatalf("32-byte words must stay intact, got %q", got)
	}
	if got := compactHex("hello"); got != "hello" {
		t.Fatalf("non-hex must be untouched, got %q", got)
	}
}

func repeatHex(unit string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += unit
	}
	return out
}

func TestCompactNumberText(t *testing.T) {
	cases := map[string]string{
		"1000000000 (1,000,000,000)":                                        "1,000,000,000",
		"amountIn 1000000000 (1,000,000,000) uint256":                       "amountIn 1,000,000,000 uint256",
		"1000000 (1 USDC)":                                                  "1 USDC",
		"2552732552244911963753134392 (2552732552.244911963753134392 PEPE)": "2,552,732,552.2449 PEPE",
		"51932126031271887 (0.051932126031271887 WETH)":                     "0.05193 WETH",
		"1234 (1.234)": "1.234",
		"115792089237316195423570985008687907853269984665640564039457584007913129639935 (uint256.max (∞), all USDC)": "uint256.max (∞) USDC",
		"18446744073709551615 (uint64.max, all X)": "uint64.max X",
		"999": "999",
		"0x3f9a1c2b4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e1c2": "0x3f9a1c2b4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e1c2",
	}
	for in, want := range cases {
		if got := compactNumberText(in); got != want {
			t.Errorf("compactNumberText(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestPackArgsWrapsToTerminalWidth(t *testing.T) {
	p := txPrinter{width: 36}
	head := "1. Transfer  USDC"
	args := []string{"from 0x9642…5D4E", "to 0x0d4a…1852", "value 1,000 USDC"}
	lines := p.packArgs(head, args, 4, 2)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines at width 36, got %d: %q", len(lines), lines)
	}
	if lines[0] != "1. Transfer  USDC   from 0x9642…5D4E" {
		t.Fatalf("first line %q", lines[0])
	}
	if lines[1] != "    to 0x0d4a…1852" || lines[2] != "    value 1,000 USDC" {
		t.Fatalf("continuation lines %q", lines[1:])
	}
	// The first argument stays with the head even when tight; continuation
	// lines must respect the width.
	for _, l := range lines[1:] {
		if w := len([]rune(l)) + 2; w > 36 {
			t.Fatalf("line exceeds width: %q (%d)", l, w)
		}
	}
	if got := (txPrinter{width: 0}).packArgs(head, args, 4, 2); len(got) != 1 {
		t.Fatalf("no width must not wrap: %q", got)
	}
}
