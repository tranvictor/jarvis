package walletconnect

import (
	"strings"
	"testing"
)

func TestChainAndAccountStrings(t *testing.T) {
	if got := ChainString(1); got != "eip155:1" {
		t.Errorf("ChainString(1) = %q", got)
	}
	if got := AccountString(137, "0xAbCdEf0000000000000000000000000000000001"); got != "eip155:137:0xabcdef0000000000000000000000000000000001" {
		t.Errorf("AccountString mixed-case not lower-cased: %q", got)
	}
}

func TestParseChain(t *testing.T) {
	id, err := ParseChain("eip155:42161")
	if err != nil || id != 42161 {
		t.Fatalf("ParseChain(eip155:42161) = %d, %v", id, err)
	}
	if _, err := ParseChain("cosmos:cosmoshub-4"); err == nil {
		t.Fatal("expected namespace rejection")
	}
	if _, err := ParseChain("eip155:notanumber"); err == nil {
		t.Fatal("expected uint parse error")
	}
	if _, err := ParseChain("eip155"); err == nil {
		t.Fatal("expected shape rejection")
	}
}

func TestParseAccount(t *testing.T) {
	id, addr, err := ParseAccount("eip155:1:0x00000000219ab540356cBB839Cbe05303d7705Fa")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id != 1 {
		t.Errorf("chain id = %d", id)
	}
	if !strings.EqualFold(addr.Hex(), "0x00000000219ab540356cBB839Cbe05303d7705Fa") {
		t.Errorf("addr = %s", addr.Hex())
	}
	if _, _, err := ParseAccount("eip155:1:not-an-addr"); err == nil {
		t.Fatal("expected address validation error")
	}
	if _, _, err := ParseAccount("eip155:1"); err == nil {
		t.Fatal("expected missing-address error")
	}
}
