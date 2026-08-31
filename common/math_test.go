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
