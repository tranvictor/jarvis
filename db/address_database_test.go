package db

import "testing"

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
