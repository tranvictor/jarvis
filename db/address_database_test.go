package db

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestGetAddressMatches_ExactAddressNeverFallsBackToFuzzy reproduces a real
// bug report: a registered address whose *label* happens to mention another
// address's hex string must not be returned when the user looks up that
// other address directly. (Addresses below are synthetic, not real
// on-chain addresses.)
func TestGetAddressMatches_ExactAddressNeverFallsBackToFuzzy(t *testing.T) {
	const (
		lookedUpAddr = "0x1111111111111111111111111111111111111111"
		decoyAddr    = "0x2222222222222222222222222222222222222222"
		lookedUpDesc = "My multisig"
		decoyDesc    = "Decoy - Test multisig same owners as " + lookedUpAddr
	)

	source := FuzzySource{
		{
			Address:      lookedUpAddr,
			Desc:         lookedUpDesc,
			SearchString: lookedUpDesc + "_" + lookedUpAddr,
		},
		{
			Address:      decoyAddr,
			Desc:         decoyDesc,
			SearchString: decoyDesc + "_" + decoyAddr,
		},
	}
	exact := func(addr string) (AddressDesc, bool) {
		for _, ad := range source {
			if ad.Address == addr {
				return ad, true
			}
		}
		return AddressDesc{}, false
	}

	results, _ := getAddressMatches(lookedUpAddr, source, exact)
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 match, got %d: %+v", len(results), results)
	}
	if results[0].Address != lookedUpAddr {
		t.Fatalf("expected %s, got %s (desc: %q) — address input was hijacked by a label match",
			lookedUpAddr, results[0].Address, results[0].Desc)
	}
}

// TestGetAddressMatches_UnregisteredAddressDoesNotFuzzyMatch reproduces the
// zero-address bug report: an address that is not registered (but is
// close, character-wise, to a registered one such as address(0)) must be
// reported as "not found" rather than fuzzy-matched to the near-miss.
func TestGetAddressMatches_UnregisteredAddressDoesNotFuzzyMatch(t *testing.T) {
	const (
		zeroAddr    = "0x0000000000000000000000000000000000000000"
		unknownAddr = "0x0000000000000000000000000000000000000003"
	)

	source := FuzzySource{
		{
			Address:      zeroAddr,
			Desc:         "Zero address",
			SearchString: "Zero_address_" + zeroAddr,
		},
	}
	exact := func(addr string) (AddressDesc, bool) {
		for _, ad := range source {
			if ad.Address == addr {
				return ad, true
			}
		}
		return AddressDesc{}, false
	}

	results, _ := getAddressMatches(unknownAddr, source, exact)
	if len(results) != 0 {
		t.Fatalf("expected no match for unregistered address, got %+v (should not fuzzy-match address(0))", results)
	}
}

// TestGetAddressMatches_FreeTextStillFuzzyMatches ensures the fix didn't
// remove fuzzy search for genuine free-text/label queries.
func TestGetAddressMatches_FreeTextStillFuzzyMatches(t *testing.T) {
	const addr = "0x1111111111111111111111111111111111111111"
	source := FuzzySource{
		{
			Address:      addr,
			Desc:         "Foobar multisig",
			SearchString: "Foobar_multisig_" + addr,
		},
	}
	exact := func(string) (AddressDesc, bool) { return AddressDesc{}, false }

	results, _ := getAddressMatches("foobar", source, exact)
	if len(results) != 1 || results[0].Address != addr {
		t.Fatalf("expected label search to still fuzzy-match, got %+v", results)
	}
}

func TestGetAddressMatchesPreservesPersonal(t *testing.T) {
	const (
		meAddr = "0x1111111111111111111111111111111111111111"
		usdc   = "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"
	)
	source := FuzzySource{
		{Address: meAddr, Desc: "me", SearchString: "me_" + meAddr, Personal: true},
		{Address: usdc, Desc: "USDC token", SearchString: "USDC_token_" + usdc, Personal: false},
	}
	exact := func(addr string) (AddressDesc, bool) {
		for _, ad := range source {
			if ad.Address == addr {
				return ad, true
			}
		}
		return AddressDesc{}, false
	}
	got, _ := getAddressMatches(meAddr, source, exact)
	if len(got) != 1 || !got[0].Personal || got[0].Desc != "me" {
		t.Fatalf("personal book entry: %+v", got)
	}
	got, _ = getAddressMatches(usdc, source, exact)
	if len(got) != 1 || got[0].Personal || got[0].Desc != "USDC token" {
		t.Fatalf("bundled token: %+v", got)
	}
}

func TestRegisterIgnoresInvalidKeysAndZeroAddress(t *testing.T) {
	d := &DefaultAddressDatabase{Data: map[common.Address]string{}}
	d.Register("Quang Le", "should not bind")
	d.Register("", "empty")
	d.Register("0x0", "short")
	d.Register("0x0000000000000000000000000000000000000000", "Quang Le")
	if _, ok := d.Data[common.Address{}]; ok {
		t.Fatalf("zero address must not carry an address-book name: %+v", d.Data)
	}
	const me = "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"
	d.Register(me, "me")
	if d.Data[common.HexToAddress(me)] != "me" {
		t.Fatal("valid addresses must still register")
	}
}
