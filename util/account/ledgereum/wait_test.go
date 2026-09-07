package ledgereum

import (
	"errors"
	"testing"
	"time"

	"github.com/tranvictor/jarvis/ui"
)

func TestUnlockWithWaitReportsThroughProgressUI(t *testing.T) {
	origPoll, origUI := deviceWaitPoll, ProgressUI
	deviceWaitPoll = time.Millisecond
	rec := ui.NewRecordingUI()
	ProgressUI = rec
	defer func() { deviceWaitPoll, ProgressUI = origPoll, origUI }()

	attempts := 0
	err := unlockWithWait(func() error {
		attempts++
		if attempts < 3 {
			return errors.New("Ledger device is not found")
		}
		return nil
	}, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var methods []string
	for _, e := range rec.Entries() {
		methods = append(methods, e.Method)
	}
	want := []string{"Spinner", "SpinnerUpdate", "SpinnerStop"}
	if len(methods) != len(want) {
		t.Fatalf("got %v, want %v", rec.Entries(), want)
	}
	for i := range want {
		if methods[i] != want[i] {
			t.Fatalf("entry %d: got %v, want %v", i, rec.Entries(), want)
		}
	}
	if !rec.HasMessage("✓ Ledger connected") || !rec.HasMessage("left)") {
		t.Fatalf("expected countdown and success line, got %v", rec.Entries())
	}
}

func TestUnlockWithWaitTimesOut(t *testing.T) {
	origPoll, origUI := deviceWaitPoll, ProgressUI
	deviceWaitPoll = time.Millisecond
	rec := ui.NewRecordingUI()
	ProgressUI = rec
	defer func() { deviceWaitPoll, ProgressUI = origPoll, origUI }()

	err := unlockWithWait(func() error { return errors.New("no such device") }, 5*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !rec.HasMessage("✗ Ledger not detected") {
		t.Fatalf("expected failure line, got %v", rec.Entries())
	}
}

func TestUnlockWithWaitSurfacesOtherErrorsImmediately(t *testing.T) {
	origUI := ProgressUI
	ProgressUI = ui.NewRecordingUI()
	defer func() { ProgressUI = origUI }()

	err := unlockWithWait(func() error { return errors.New("user rejected") }, time.Second)
	if err == nil || err.Error() != "user rejected" {
		t.Fatalf("got %v", err)
	}
}
