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
	})

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
