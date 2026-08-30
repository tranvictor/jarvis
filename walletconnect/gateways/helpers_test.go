package gateways

import (
	"errors"
	"testing"

	"github.com/tranvictor/jarvis/walletconnect"
)

func TestErrMethodUnsupportedWrapsSentinel(t *testing.T) {
	err := errMethodUnsupported("a Safe cannot produce an EOA personal_sign signature")
	if !errors.Is(err, walletconnect.ErrMethodNotSupported) {
		t.Fatalf("session errorCodeFor would map this to 5000 instead of 5101: %v", err)
	}
}

func TestErrSwitchChainPinnedWrapsSentinel(t *testing.T) {
	err := errSwitchChainPinned("Safe sessions are pinned to the chain they opened on")
	if !errors.Is(err, walletconnect.ErrChainNotSupported) {
		t.Fatalf("session errorCodeFor would map this to 5000 instead of 5100: %v", err)
	}
}

func TestVerifyOwner(t *testing.T) {
	owners := []string{"0xAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAa"}
	if err := verifyOwner("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", owners, "Safe", "0xsafe"); err != nil {
		t.Fatalf("checksum vs lower should match: %v", err)
	}
	err := verifyOwner("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", owners, "Safe", "0xsafe")
	if err == nil {
		t.Fatal("expected reject")
	}
	if want := "wallet 0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb is not an owner of Safe 0xsafe"; err.Error() != want {
		t.Fatalf("got %q", err.Error())
	}
}
