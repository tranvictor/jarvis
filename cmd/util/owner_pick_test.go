package util

import (
	"errors"
	"strings"
	"testing"

	jtypes "github.com/tranvictor/jarvis/accounts/types"
)

func testLookup(wallets map[string]jtypes.AccDesc) accountLookup {
	return func(addr string) (jtypes.AccDesc, error) {
		acc, ok := wallets[addr]
		if !ok {
			return jtypes.AccDesc{}, errors.New("not found")
		}
		return acc, nil
	}
}

func TestPickLocalOwnerRequireUnique(t *testing.T) {
	alice := jtypes.AccDesc{Address: "0xAAA"}
	bob := jtypes.AccDesc{Address: "0xBBB"}
	lookup := testLookup(map[string]jtypes.AccDesc{
		"0xaaa": alice,
		"0xbbb": bob,
	})

	_, _, err := pickLocalOwner([]string{"0xccc"}, "", OwnerRequireUnique, lookup)
	if !errors.Is(err, ErrNoLocalOwner) {
		t.Fatalf("empty: got %v, want ErrNoLocalOwner", err)
	}

	got, n, err := pickLocalOwner([]string{"0xaaa", "0xccc"}, "", OwnerRequireUnique, lookup)
	if err != nil || n != 1 || got.Address != "0xAAA" {
		t.Fatalf("one match: acc=%+v n=%d err=%v", got, n, err)
	}

	_, n, err = pickLocalOwner([]string{"0xaaa", "0xbbb"}, "", OwnerRequireUnique, lookup)
	if !errors.Is(err, ErrMultipleLocalOwners) || n != 2 {
		t.Fatalf("two matches: n=%d err=%v", n, err)
	}
}

func TestPickLocalOwnerFirstMatch(t *testing.T) {
	alice := jtypes.AccDesc{Address: "0xAAA"}
	bob := jtypes.AccDesc{Address: "0xBBB"}
	lookup := testLookup(map[string]jtypes.AccDesc{
		"0xaaa": alice,
		"0xbbb": bob,
	})

	got, n, err := pickLocalOwner([]string{"0xaaa", "0xbbb"}, "", OwnerFirstMatch, lookup)
	if err != nil || n != 2 || got.Address != "0xAAA" {
		t.Fatalf("first match: acc=%+v n=%d err=%v", got, n, err)
	}
}

func TestPickLocalOwnerEmptyOwners(t *testing.T) {
	lookup := testLookup(map[string]jtypes.AccDesc{
		"0xaaa": {Address: "0xAAA"},
	})
	_, n, err := pickLocalOwner(nil, "", OwnerFirstMatch, lookup)
	if !errors.Is(err, ErrNoLocalOwner) || n != 0 {
		t.Fatalf("nil owners: n=%d err=%v", n, err)
	}
	_, n, err = pickLocalOwner([]string{}, "", OwnerRequireUnique, lookup)
	if !errors.Is(err, ErrNoLocalOwner) || n != 0 {
		t.Fatalf("empty owners: n=%d err=%v", n, err)
	}
}

func TestPickLocalOwnerSkipsFailedLookups(t *testing.T) {
	lookup := testLookup(map[string]jtypes.AccDesc{
		"0xbbb": {Address: "0xBBB"},
	})
	got, n, err := pickLocalOwner([]string{"0xaaa", "0xbbb", "0xccc"}, "", OwnerRequireUnique, lookup)
	if err != nil || n != 1 || got.Address != "0xBBB" {
		t.Fatalf("acc=%+v n=%d err=%v", got, n, err)
	}
}

func TestChooseSafeFromExecuteAllowsNonOwner(t *testing.T) {
	alice := jtypes.AccDesc{Address: "0xAAA"}
	carol := jtypes.AccDesc{Address: "0xCCC"}
	lookup := testLookup(map[string]jtypes.AccDesc{
		"0xaaa": alice,
		"0xccc": carol,
	})
	resolve := func(keyword string) (jtypes.AccDesc, error) {
		return lookup(keyword)
	}
	owners := []string{"0xaaa"}

	got, err := chooseSafeFrom("0xccc", "0xsafe", owners, false, resolve, lookup, nil)
	if err != nil || got.Address != "0xCCC" {
		t.Fatalf("execute --from non-owner: acc=%+v err=%v", got, err)
	}

	_, err = chooseSafeFrom("0xccc", "0xsafe", owners, true, resolve, lookup, nil)
	if err == nil || !strings.Contains(err.Error(), "is not an owner of Safe 0xsafe") {
		t.Fatalf("approve --from non-owner: err=%v", err)
	}
}

func TestChooseSafeFromExecuteFallsBackToUniqueWallet(t *testing.T) {
	carol := jtypes.AccDesc{Address: "0xCCC"}
	lookup := testLookup(nil)
	resolve := func(string) (jtypes.AccDesc, error) {
		t.Fatal("should not resolve --from")
		return jtypes.AccDesc{}, nil
	}

	got, err := chooseSafeFrom("", "0xsafe", []string{"0xaaa"}, false, resolve, lookup, map[string]jtypes.AccDesc{
		"0xccc": carol,
	})
	if err != nil || got.Address != "0xCCC" {
		t.Fatalf("execute with one non-owner wallet: acc=%+v err=%v", got, err)
	}

	_, err = chooseSafeFrom("", "0xsafe", []string{"0xaaa"}, false, resolve, lookup, map[string]jtypes.AccDesc{
		"0xccc": carol,
		"0xddd": {Address: "0xDDD"},
	})
	if err == nil || !strings.Contains(err.Error(), "multiple local wallets") {
		t.Fatalf("execute with many wallets: err=%v", err)
	}

	_, err = chooseSafeFrom("", "0xsafe", []string{"0xaaa"}, false, resolve, lookup, nil)
	if err == nil || !strings.Contains(err.Error(), "no local wallet to pay for execution") {
		t.Fatalf("execute with no wallets: err=%v", err)
	}
}

func TestChooseSafeFromPrefersUniqueOwner(t *testing.T) {
	alice := jtypes.AccDesc{Address: "0xAAA"}
	lookup := testLookup(map[string]jtypes.AccDesc{"0xaaa": alice})
	got, err := chooseSafeFrom("", "0xsafe", []string{"0xaaa"}, false, nil, lookup, map[string]jtypes.AccDesc{
		"0xaaa": alice,
		"0xccc": {Address: "0xCCC"},
	})
	if err != nil || got.Address != "0xAAA" {
		t.Fatalf("unique owner should win even when other wallets exist: acc=%+v err=%v", got, err)
	}
}

func TestPickUniqueLocalWallet(t *testing.T) {
	_, err := pickUniqueLocalWallet(nil)
	if !errors.Is(err, ErrNoLocalWallet) {
		t.Fatalf("empty: %v", err)
	}
	got, err := pickUniqueLocalWallet(map[string]jtypes.AccDesc{"0xccc": {Address: "0xCCC"}})
	if err != nil || got.Address != "0xCCC" {
		t.Fatalf("one: acc=%+v err=%v", got, err)
	}
	_, err = pickUniqueLocalWallet(map[string]jtypes.AccDesc{
		"0xccc": {Address: "0xCCC"},
		"0xddd": {Address: "0xDDD"},
	})
	if !errors.Is(err, ErrMultipleLocalWallets) {
		t.Fatalf("many: %v", err)
	}
}

func TestIsAmongOwners(t *testing.T) {
	owners := []string{"0xAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAa"}
	if !IsAmongOwners(owners, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Fatal("expected case-insensitive match")
	}
	if IsAmongOwners(owners, "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb") {
		t.Fatal("unexpected match")
	}
	if IsAmongOwners(nil, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Fatal("empty owner list must not match")
	}
}
