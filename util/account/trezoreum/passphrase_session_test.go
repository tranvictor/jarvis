package trezoreum

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestLookupDoesNotUsePendingWhenAddressIsSet(t *testing.T) {
	resetPassphraseSessionForTest()
	t.Cleanup(resetPassphraseSessionForTest)

	rememberPending("hidden-a", false)
	want := common.HexToAddress("0x1111111111111111111111111111111111111111")
	other := common.HexToAddress("0x2222222222222222222222222222222222222222")

	if _, ok := lookupCachedPassphrase(want); ok {
		t.Fatal("pending must not be reused for an unbound expected address")
	}

	bindPassphrase(other)
	if _, ok := lookupCachedPassphrase(want); ok {
		t.Fatal("secret bound to another address must not be reused")
	}

	e, ok := lookupCachedPassphrase(other)
	if !ok || e.secret != "hidden-a" || e.onDevice {
		t.Fatalf("bound lookup: got %+v ok=%v", e, ok)
	}
}

func TestLookupPendingWhenNoExpectedAddress(t *testing.T) {
	resetPassphraseSessionForTest()
	t.Cleanup(resetPassphraseSessionForTest)

	rememberPending("wallet-add", true)
	e, ok := lookupCachedPassphrase(common.Address{})
	if !ok || e.secret != "wallet-add" || !e.onDevice {
		t.Fatalf("pending lookup: got %+v ok=%v", e, ok)
	}
}

func TestResetPassphraseSessionClearsPendingAndSession(t *testing.T) {
	resetPassphraseSessionForTest()
	t.Cleanup(resetPassphraseSessionForTest)

	storeSessionID([]byte{1, 2, 3})
	rememberPending("secret", false)
	addr := common.HexToAddress("0x3333333333333333333333333333333333333333")
	bindPassphrase(addr)

	ResetPassphraseSession(addr)

	if currentSessionID() != nil {
		t.Fatal("session id should be cleared")
	}
	if _, ok := lookupCachedPassphrase(common.Address{}); ok {
		t.Fatal("pending should be cleared")
	}
	if _, ok := lookupCachedPassphrase(addr); ok {
		t.Fatal("address binding should be cleared")
	}
}

func TestPassphraseMaskHidesEmptyAndNonEmpty(t *testing.T) {
	if passphraseMask != "********" {
		t.Fatalf("mask must be fixed-length, got %q", passphraseMask)
	}
}
