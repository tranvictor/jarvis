package util

import (
	"errors"
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
