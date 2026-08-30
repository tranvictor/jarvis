package safe

import (
	"testing"

	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util/reader"
)

func TestNewSafeContractWithReader(t *testing.T) {
	r := &reader.EthReader{}
	sc, err := NewSafeContract("0xabc", networks.EthereumMainnet, WithReader(r))
	if err != nil {
		t.Fatal(err)
	}
	if sc.reader != r {
		t.Fatal("injected reader was not used")
	}
	if sc.Address != "0xabc" {
		t.Fatalf("address %q", sc.Address)
	}
	if sc.Abi == nil {
		t.Fatal("expected Safe ABI")
	}
}

func TestWithReaderNilIsIgnored(t *testing.T) {
	var o contractOptions
	WithReader(nil)(&o)
	if o.reader != nil {
		t.Fatal("nil reader should be ignored")
	}
}
