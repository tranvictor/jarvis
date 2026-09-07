package common

import (
	"math/big"
	"testing"
)

func TestGweiToWeiTypicalGasPrice(t *testing.T) {
	// 37.500044782 Gwei is a normal Ethereum gas price. Converting to wei
	// yields 37500044782, which overflows 32-bit int (strconv.Atoi) but fits
	// in int64.
	got := GweiToWei(37.500044782)
	want := big.NewInt(37500044782)
	if got.Cmp(want) != 0 {
		t.Errorf("GweiToWei(37.500044782) = %s, want %s", got, want)
	}
}

func TestFloatToIntBeyondInt32(t *testing.T) {
	got := FloatToInt(37500044782)
	const want int64 = 37500044782
	if got != want {
		t.Errorf("FloatToInt(37500044782) = %d, want %d", got, want)
	}
}

func TestFloatStringToBigIsExact(t *testing.T) {
	cases := []struct {
		in       string
		decimals uint64
		want     string
	}{
		{"0.001", 6, "1000"},
		{"0.1", 18, "100000000000000000"},
		{"1", 18, "1000000000000000000"},
		{"1,000.5", 6, "1000500000"},
		{"1_000", 0, "1000"},
		{"1e6", 0, "1000000"},
		{"0.0000001", 6, "0"}, // below precision truncates, never rounds up
		{"123456789.123456789", 9, "123456789123456789"},
	}
	for _, c := range cases {
		got, err := FloatStringToBig(c.in, c.decimals)
		if err != nil {
			t.Fatalf("%s: %s", c.in, err)
		}
		if got.String() != c.want {
			t.Errorf("%s with %d decimals = %s, want %s", c.in, c.decimals, got, c.want)
		}
	}
	for _, bad := range []string{"", "abc", "1.2.3"} {
		if _, err := FloatStringToBig(bad, 18); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
}
