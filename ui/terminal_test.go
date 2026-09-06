package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestStyleMutedUsesFaint(t *testing.T) {
	u := NewTerminalUIWithWriter(&bytes.Buffer{}, true)
	got := u.Style(StyledText{Text: "uint256", Severity: SeverityMuted})
	if !strings.Contains(got, "\x1b[2m") {
		t.Fatalf("expected faint SGR in %q", got)
	}
	plain := NewTerminalUIWithWriter(&bytes.Buffer{}, false)
	if got := plain.Style(StyledText{Text: "uint256", Severity: SeverityMuted}); got != "uint256" {
		t.Fatalf("expected plain text when colours are off, got %q", got)
	}
}

func TestKeyValueCellsAlignsOnVisibleWidth(t *testing.T) {
	var buf bytes.Buffer
	u := NewTerminalUIWithWriter(&buf, true)
	u.KeyValueCells([][2]TableCell{
		{TCS("Sign with", SeverityMuted), TC("0xabc")},
		{TCS("To", SeverityMuted), TCS("0xdef", SeveritySuccess)},
	})
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), buf.String())
	}
	col0 := strings.Index(stripANSI(lines[0]), "0xabc")
	col1 := strings.Index(stripANSI(lines[1]), "0xdef")
	if col0 != col1 {
		t.Fatalf("values not aligned: %d vs %d in\n%s", col0, col1, stripANSI(buf.String()))
	}
	if !strings.Contains(lines[1], "\x1b[32m") {
		t.Fatalf("expected green value, got %q", lines[1])
	}
}

func TestSubsectionAndRewriteOffTTY(t *testing.T) {
	var buf bytes.Buffer
	u := NewTerminalUIWithWriter(&buf, false)
	u.Subsection("Transfers")
	u.Info("first")
	u.RewriteLastLine("second")
	want := "\nTransfers\nfirst\nsecond\n"
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestRewriteOnTTYMovesCursorUp(t *testing.T) {
	var buf bytes.Buffer
	u := NewTerminalUIWithWriter(&buf, false)
	u.tty = true
	u.Indent().RewriteLastLine("done")
	if got := buf.String(); got != "\x1b[1A\x1b[2K  done\n" {
		t.Fatalf("got %q", got)
	}
}

func TestSpinnerOffTTYPrintsDistinctMessagesOnce(t *testing.T) {
	var buf bytes.Buffer
	u := NewTerminalUIWithWriter(&buf, false)
	p := u.Spinner("waiting")
	p.Update("waiting")
	p.Update("in mempool")
	p.Stop(StyledText{Text: "✓ mined", Severity: SeveritySuccess})
	p.Stop(StyledText{Text: "again"}) // second Stop is ignored
	want := "waiting\nin mempool\n✓ mined\n"
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
	if p.Elapsed() < 0 || p.Elapsed() > time.Minute {
		t.Fatalf("unexpected elapsed %v", p.Elapsed())
	}
}

func TestSpinnerStopWithEmptyFinalPrintsNothing(t *testing.T) {
	var buf bytes.Buffer
	u := NewTerminalUIWithWriter(&buf, false)
	p := u.Spinner("fetching")
	p.Stop(StyledText{})
	if buf.String() != "fetching\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestFormatElapsed(t *testing.T) {
	cases := map[time.Duration]string{
		0:                              "0:00",
		14 * time.Second:               "0:14",
		61 * time.Second:               "1:01",
		10*time.Minute + 5*time.Second: "10:05",
	}
	for d, want := range cases {
		if got := formatElapsed(d); got != want {
			t.Errorf("formatElapsed(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestRecordingSpinnerRecordsStates(t *testing.T) {
	r := NewRecordingUI()
	p := r.Spinner("waiting")
	p.Update("in mempool")
	p.Stop(StyledText{Text: "✓ mined"})
	got := r.Entries()
	want := []Entry{
		{"Spinner", "waiting"},
		{"SpinnerUpdate", "in mempool"},
		{"SpinnerStop", "✓ mined"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestTreeGlyphs(t *testing.T) {
	if TreeBranch(false) != "├─ " || TreeBranch(true) != "└─ " {
		t.Fatal("unexpected branch glyphs")
	}
	if TreeGuide(false) != "│  " || TreeGuide(true) != "   " {
		t.Fatal("unexpected guide glyphs")
	}
	if VisibleWidth(TreeBranch(false)) != VisibleWidth(TreeGuide(false)) {
		t.Fatal("branch and guide must have equal width so nested lines align")
	}
}

func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		case r == '\x1b':
			inEsc = true
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
