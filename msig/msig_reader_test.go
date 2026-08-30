package msig

import (
	"testing"

	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util/reader"
)

func TestNewMultisigContractWithReader(t *testing.T) {
	r := &reader.EthReader{}
	mc, err := NewMultisigContract("0xabc", networks.EthereumMainnet, WithReader(r))
	if err != nil {
		t.Fatal(err)
	}
	if mc.reader != r {
		t.Fatal("injected reader was not used")
	}
	if mc.Address != "0xabc" {
		t.Fatalf("address %q", mc.Address)
	}
	if mc.Abi == nil {
		t.Fatal("expected Classic ABI")
	}
}

func TestWithReaderNilIsIgnored(t *testing.T) {
	var o contractOptions
	WithReader(nil)(&o)
	if o.reader != nil {
		t.Fatal("nil reader should be ignored")
	}
}
