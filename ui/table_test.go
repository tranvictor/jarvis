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

func TestRenderTableRightAlignsColumn(t *testing.T) {
	var buf bytes.Buffer
	renderTable(&buf, "", &Table{
		Headers: []string{"token", "value", "note"},
		Align:   []ColAlign{AlignLeft, AlignRight, AlignLeft},
		Rows: [][]TableCell{
			{TC("ETH"), TCS("1,234.56", SeverityInfo), TC("sell")},
			{TC("USDC"), TCS("▲ 12,345.00", SeveritySuccess), TC("live")},
			{TC("DAI"), TCS("9.50", SeverityError), TC("thin")},
			{TC("??"), TCS("—", SeverityMuted), TC("none")},
		},
	}, func(cell TableCell) string {
		switch cell.Severity {
		case SeveritySuccess:
			return "\x1b[32m" + cell.Text + "\x1b[0m"
		case SeverityError:
			return "\x1b[31m" + cell.Text + "\x1b[0m"
		default:
			return cell.Text
		}
	}, 0)

	plain := ansi.Strip(buf.String())
	var valueCells []string
	var tokenCells []string
	for _, line := range strings.Split(strings.TrimRight(plain, "\n"), "\n") {
		parts := strings.Split(line, "│")
		if len(parts) < 4 {
			continue
		}
		tokenCells = append(tokenCells, parts[1])
		valueCells = append(valueCells, parts[2])
	}
	if len(valueCells) != 5 {
		t.Fatalf("expected header + 4 value cells, got %d:\n%s", len(valueCells), plain)
	}
	for i, cell := range valueCells {
		if !rightAligned(cell) {
			t.Fatalf("value cell %d is not right-aligned: %q\n%s", i, cell, plain)
		}
	}
	// The header and every amount share the column's right edge, so the
	// cents of "1,234.56" and "9.50" sit in the same columns.
	if !strings.HasSuffix(strings.TrimRight(valueCells[1], " "), "1,234.56") ||
		!strings.HasSuffix(strings.TrimRight(valueCells[3], " "), "9.50") ||
		!strings.HasSuffix(strings.TrimRight(valueCells[2], " "), "▲ 12,345.00") {
		t.Fatalf("value text lost:\n%s", plain)
	}
	for i, cell := range tokenCells {
		if !leftAligned(cell) {
			t.Fatalf("token cell %d changed alignment: %q\n%s", i, cell, plain)
		}
	}
	// Colour stays on the glyphs, not on the padding that lines the column up.
	raw := buf.String()
	if !strings.Contains(raw, "\x1b[32m▲ 12,345.00\x1b[0m") {
		t.Fatalf("green tick was padded inside the colour:\n%s", raw)
	}
	if !strings.Contains(raw, "\x1b[31m9.50\x1b[0m") {
		t.Fatalf("red value lost its colour:\n%s", raw)
	}
}

// rightAligned reports that a bordered cell (" "+text+" ") has its text
// flush with the right padding space.
func rightAligned(cell string) bool {
	if len(cell) < 2 || cell[0] != ' ' || cell[len(cell)-1] != ' ' {
		return false
	}
	inner := cell[1 : len(cell)-1]
	return inner == "" || inner[len(inner)-1] != ' '
}

func leftAligned(cell string) bool {
	if len(cell) < 2 || cell[0] != ' ' || cell[len(cell)-1] != ' ' {
		return false
	}
	inner := cell[1 : len(cell)-1]
	return inner == "" || inner[0] != ' '
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
