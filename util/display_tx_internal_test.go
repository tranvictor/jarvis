package util

import "testing"

func TestCompactAddressText(t *testing.T) {
	const a = "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"
	const b = "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"
	const hash = "0x3f9a1c2b4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e1c2"

	cases := map[string]string{
		a:                                 a[:6] + "…5D4E (unknown)",
		a + " (me)":                       "me (0x9642…5D4E)",
		a + " (USDC - 6)":                 "USDC (0x9642…5D4E)",
		a + " (unknown)":                  "0x9642…5D4E (unknown)",
		"from " + a + " to " + b:          "from 0x9642…5D4E (unknown) to 0x7a25…488D (unknown)",
		a + "," + b:                       "0x9642…5D4E (unknown),0x7a25…488D (unknown)",
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
