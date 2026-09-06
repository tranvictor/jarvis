package util

import (
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/ui"
)

func TestWaitForStatusesTracksMempoolThenOutcome(t *testing.T) {
	cases := []struct {
		statuses []string
		final    string
		stop     string
	}{
		{[]string{"pending", "done"}, "done", "✓ mined after"},
		{[]string{"done"}, "done", "✓ mined after"},
		{[]string{"pending", "reverted"}, "reverted", "✗ reverted after"},
		{[]string{"pending", "lost"}, "lost", "✗ dropped from the mempool after"},
	}
	for _, c := range cases {
		ch := make(chan string, len(c.statuses))
		for _, s := range c.statuses {
			ch <- s
		}
		close(ch)

		rec := ui.NewRecordingUI()
		got := waitForStatuses(rec, ch)
		if got != c.final {
			t.Errorf("%v: final %q, want %q", c.statuses, got, c.final)
		}
		var methods []string
		for _, e := range rec.Entries() {
			methods = append(methods, e.Method+": "+e.Value)
		}
		joined := strings.Join(methods, "\n")
		if !strings.HasPrefix(joined, "Spinner: waiting for the tx to show up in the mempool…") {
			t.Errorf("%v: spinner should start before the mempool check:\n%s", c.statuses, joined)
		}
		if c.statuses[0] == "pending" && !strings.Contains(joined, "SpinnerUpdate: in mempool, waiting to be mined…") {
			t.Errorf("%v: expected mempool update:\n%s", c.statuses, joined)
		}
		if !strings.Contains(joined, "SpinnerStop: "+c.stop) {
			t.Errorf("%v: expected final line %q:\n%s", c.statuses, c.stop, joined)
		}
	}
}

func TestWaitForStatusesClosedWithoutOutcome(t *testing.T) {
	ch := make(chan string)
	close(ch)
	rec := ui.NewRecordingUI()
	if got := waitForStatuses(rec, ch); got != "unknown" {
		t.Fatalf("got %q", got)
	}
	if !rec.HasMessage("✗ stopped waiting") {
		t.Fatalf("expected a stop line, got %v", rec.Entries())
	}
}
