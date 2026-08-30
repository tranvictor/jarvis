package util

import (
	"testing"

	"github.com/tranvictor/jarvis/util/reader"
)

func TestEthReaderOf(t *testing.T) {
	if EthReaderOf(nil) != nil {
		t.Fatal("nil interface")
	}

	r := &reader.EthReader{}
	if EthReaderOf(r) != r {
		t.Fatal("concrete reader")
	}
}
