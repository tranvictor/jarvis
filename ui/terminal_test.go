package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

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

func TestKeyValueCellsWrapsLongAddressName(t *testing.T) {
	long := "0x2515ec2104d30a073c0ea99d916b9e20b14fc7e0 (Lumen Main Multisig (Mike, Victor, Loi) – Eth, Bsc, Arb, Opt, Base, Pol, Robin, Avax, Bera, S, Plasma, Monad)"
	var buf bytes.Buffer
	u := NewTerminalUIWithWriter(&buf, false)
	u.width = 64
	u.KeyValueCells([][2]TableCell{
		{TCS("Network", SeverityMuted), TC("mainnet")},
		{TCS("Multisig", SeverityMuted), TCS(long, SeveritySuccess)},
	})
	plain := stripANSI(buf.String())
	lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected wrapped Multisig value, got %d lines:\n%s", len(lines), plain)
	}
	for i, line := range lines {
		if VisibleWidth(line) > 64 {
			t.Fatalf("line %d is %d cols, want ≤ 64:\n%q", i, VisibleWidth(line), line)
		}
	}
	valueCol := strings.Index(lines[0], "mainnet")
	if valueCol < 0 {
		t.Fatalf("network value missing:\n%s", plain)
	}
	hung := 0
	for _, line := range lines[1:] {
		trim := strings.TrimLeft(line, " ")
		if trim == "" || strings.HasPrefix(trim, "Multisig") {
			continue
		}
		start := len(line) - len(trim)
		if start != valueCol {
			t.Fatalf("continuation not hung under the value column (%d vs %d):\n%s", start, valueCol, plain)
		}
		hung++
	}
	if hung == 0 {
		t.Fatalf("expected hung continuation lines:\n%s", plain)
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
