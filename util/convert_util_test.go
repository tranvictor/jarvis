package util

import (
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/networks"
)

func TestConvertToBigAcceptsIntegerSpellings(t *testing.T) {
	cases := map[string]string{
		"1000000":   "1000000",
		"1e6":       "1000000",
		"1_000_000": "1000000",
		"1,000,000": "1000000",
		"0xf4240":   "1000000",
		"2.5 ETH":   "2500000000000000000",
		"0.001 ETH": "1000000000000000",
	}
	for in, want := range cases {
		got, err := ConvertToBig(in, networks.EthereumMainnet)
		if err != nil {
			t.Fatalf("%q: %s", in, err)
		}
		if got.String() != want {
			t.Errorf("ConvertToBig(%q) = %s, want %s", in, got, want)
		}
	}
	_, err := ConvertToBig("1.5", networks.EthereumMainnet)
	if err == nil || !strings.Contains(err.Error(), "1.5 ETH") {
		t.Fatalf("a bare fraction must be rejected with a hint, got %v", err)
	}
}

func TestIsDelegationDesignator(t *testing.T) {
	addr := make([]byte, 20)
	if !IsDelegationDesignator(append([]byte{0xef, 0x01, 0x00}, addr...)) {
		t.Fatal("0xef0100||address is a 7702 designator")
	}
	if IsDelegationDesignator([]byte{0x60, 0x80, 0x60, 0x40}) || IsDelegationDesignator(nil) {
		t.Fatal("ordinary bytecode / empty code are not designators")
	}
}
