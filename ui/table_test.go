package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderTableWrapsStyledCellOnEveryLine(t *testing.T) {
	long := strings.Repeat("x", 40) + " (Victor trezor)"
	var buf bytes.Buffer
	renderTable(&buf, "", &Table{
		Headers:      []string{"Field", "Value"},
		MaxCellWidth: 20,
		Rows: [][]TableCell{
			{TC("To"), TCS(long, SeveritySuccess)},
		},
	}, func(cell TableCell) string {
		if cell.Severity == SeveritySuccess {
			return "\x1b[32m" + cell.Text + "\x1b[0m"
		}
		return cell.Text
	}, 0)

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var valueLines []string
	for _, line := range lines {
		if !strings.Contains(line, "│") {
			continue
		}
		parts := strings.Split(line, "│")
		if len(parts) < 3 {
			continue
		}
		value := strings.TrimSpace(parts[2])
		if value == "" || value == "Value" {
			continue
		}
		valueLines = append(valueLines, value)
	}
	if len(valueLines) < 2 {
		t.Fatalf("expected wrapped value lines, got %d lines in output:\n%s", len(valueLines), out)
	}
	for i, line := range valueLines {
		if !strings.Contains(line, "\x1b[32m") {
			t.Fatalf("line %d missing green SGR: %q (plain: %q)", i, line, ansi.Strip(line))
		}
	}
}

func TestWrapCellPrefersWordBreak(t *testing.T) {
	got := wrapCell("0xabc (Lumen Main Multisig Eth Bsc Arb)", 24)
	if len(got) < 2 {
		t.Fatalf("expected wrap, got %v", got)
	}
	for _, line := range got {
		if strings.HasSuffix(line, " ") {
			t.Fatalf("wrapped line should not trail a space: %q", line)
		}
		if VisibleWidth(line) > 24 {
			t.Fatalf("line %q is %d cols", line, VisibleWidth(line))
		}
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "Lumen") || !strings.Contains(joined, "Arb") {
		t.Fatalf("lost words: %v", got)
	}
}

func TestWrapCellHexHasNoSpaces(t *testing.T) {
	hex := "0x" + strings.Repeat("ab", 40)
	got := wrapCell(hex, 20)
	if len(got) < 2 {
		t.Fatalf("expected char wrap, got %v", got)
	}
	if got[0] != hex[:20] {
		t.Fatalf("hex should wrap on a column: %q", got[0])
	}
}

func TestRenderTableFitsAvailableWidth(t *testing.T) {
	long := "0x2515ec2104d30a073c0ea99d916b9e20b14fc7e0 (Lumen Main Multisig (Mike, Victor, Loi) – Eth, Bsc, Arb, Opt, Base)"
	var buf bytes.Buffer
	renderTable(&buf, "", &Table{
		Headers: []string{"Field", "Value"},
		Rows: [][]TableCell{
			{TC("Multisig"), TC(long)},
		},
	}, func(cell TableCell) string { return cell.Text }, 56)
	plain := ansi.Strip(buf.String())
	for i, line := range strings.Split(strings.TrimRight(plain, "\n"), "\n") {
		if VisibleWidth(line) > 56 {
			t.Fatalf("line %d is %d cols, want ≤ 56:\n%q\n%s", i, VisibleWidth(line), line, plain)
		}
	}
}
