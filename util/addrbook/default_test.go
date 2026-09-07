package addrbook

import (
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/networks"
)

func TestResolveZeroAddressIgnoresAddressBook(t *testing.T) {
	r := NewDefault(networks.EthereumMainnet)
	got := r.Resolve("0x0000000000000000000000000000000000000000")
	if got.Desc != "zero address" {
		t.Fatalf("zero address desc = %q, want %q", got.Desc, "zero address")
	}
	if !strings.EqualFold(got.Address, "0x0000000000000000000000000000000000000000") {
		t.Fatalf("zero address hex = %q", got.Address)
	}
}
