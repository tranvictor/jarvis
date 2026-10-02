package trezoreum

import (
	"errors"
	"strings"
	"testing"
)

// A Trezor another handle (Trezor Suite, a stale jarvis signer) has claimed
// is connected; telling the user to plug it in sends them the wrong way.
func TestUnlockWithWaitReportsBusyTrezorAsInUse(t *testing.T) {
	busy := errors.New("Couldn't open trezor device: failed to claim interface: libusb: device or resource busy [code -6]")
	err := unlockWithWait(func() error { return busy }, 0)
	if err == nil {
		t.Fatal("busy device must fail once the wait is over")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "in use") {
		t.Fatalf("busy error should say the Trezor is in use: %s", err)
	}
	if strings.Contains(msg, "not connected") || strings.Contains(msg, "plug it in") {
		t.Fatalf("busy error must not claim the Trezor is unplugged: %s", err)
	}
}

func TestUnlockWithWaitReportsMissingTrezorAsNotConnected(t *testing.T) {
	missing := errors.New("Couldn't find any trezor devices")
	err := unlockWithWait(func() error { return missing }, 0)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "not connected") {
		t.Fatalf("missing device should say not connected: %v", err)
	}
}
