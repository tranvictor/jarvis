package accounts

import (
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/accounts/types"
)

func TestLookupFindsWalletByAnyHexCasing(t *testing.T) {
	const addr = "0xa3759774994F5012E5d725dCC1B96750945C793f"
	SetWalletsForTest(map[string]types.AccDesc{
		strings.ToLower(addr): {Address: addr, Kind: "ledger", Desc: "work ledger"},
	})
	t.Cleanup(func() { SetWalletsForTest(nil) })

	got, ok := Lookup(addr)
	if !ok || got.Desc != "work ledger" || got.Kind != "ledger" {
		t.Fatalf("Lookup(%s) = %+v ok=%v", addr, got, ok)
	}
	got, ok = Lookup(strings.ToLower(addr))
	if !ok || got.Desc != "work ledger" {
		t.Fatalf("lowercase Lookup = %+v ok=%v", got, ok)
	}
	if _, ok := Lookup("0x0000000000000000000000000000000000000001"); ok {
		t.Fatal("unknown address must not look like a wallet")
	}
}

func TestLookupIgnoresRecordsWithoutKind(t *testing.T) {
	const addr = "0x69694c738fe96c41ecce3588fa5759d717cf9cc6"
	SetWalletsForTest(map[string]types.AccDesc{
		addr: {Address: addr, Desc: "not a wallet json"},
	})
	t.Cleanup(func() { SetWalletsForTest(nil) })
	if _, ok := Lookup(addr); ok {
		t.Fatal("JSON without Kind is not a Jarvis wallet")
	}
}
