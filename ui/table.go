package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
	runewidth "github.com/mattn/go-runewidth"
)

const (
	cellPadding  = 1  // spaces on each side of cell text within a column
	maxCellWidth = 55 // hard cap on any single column's visual width
)

// TableCell is a single cell in a Table with optional severity styling.
type TableCell struct {
	Text     string
	Severity Severity
}

// TC constructs a plain (Info-severity) TableCell.
func TC(text string) TableCell { return TableCell{Text: text} }

// TCS constructs a TableCell with an explicit Severity for colour rendering.
func TCS(text string, s Severity) TableCell { return TableCell{Text: text, Severity: s} }

// Table is a bordered text table.
//
// Rows are visually grouped by the first-column value: the first row of each
// group shows the value; subsequent rows with the same first-column value
// display an empty first cell. A thin horizontal rule is drawn between groups.
type Table struct {
	Headers []string
	Rows    [][]TableCell
}

// AddRow appends a row of cells to the table.
func (t *Table) AddRow(cells ...TableCell) {
	t.Rows = append(t.Rows, cells)
}

// runeLen returns the visible terminal width of s.
// It strips ANSI escape codes first, then uses runewidth to correctly account
// for multi-byte characters (✓, ─, …) and East Asian wide characters.
func runeLen(s string) int {
	return runewidth.StringWidth(ansi.Strip(s))
}

// colWidths returns the minimum column widths (in runes/visual chars) required
// to display all content, capped at maxCellWidth.
func colWidths(t *Table) []int {
	n := len(t.Headers)
	widths := make([]int, n)
	for i, h := range t.Headers {
		widths[i] = runeLen(h)
	}
	for _, row := range t.Rows {
		for i, cell := range row {
			if i >= n {
				break
			}
			w := runeLen(cell.Text)
			if w > maxCellWidth {
				w = maxCellWidth
			}
			if w > widths[i] {
				widths[i] = w
			}
		}
	}
	return widths
}

// truncateStr truncates s to at most maxRunes Unicode code points, appending
// "..." when truncation occurs (preserving rune boundaries).
func truncateStr(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

// renderTable writes a full bordered table to out.
// styleCell is invoked for every data cell (not headers) and may wrap the
// cell's text in ANSI escape sequences; the caller decides colour on/off.
// Padding is computed from the plain text length so ANSI codes don't disrupt
// column alignment.
func renderTable(out io.Writer, t *Table, styleCell func(TableCell) string) {
	if len(t.Headers) == 0 {
		return
	}
	widths := colWidths(t)
	n := len(widths)

	hline := func(left, mid, right, fill string) {
		fmt.Fprint(out, left)
		for i, w := range widths {
			fmt.Fprint(out, strings.Repeat(fill, w+2*cellPadding))
			if i < n-1 {
				fmt.Fprint(out, mid)
			}
		}
		fmt.Fprintln(out, right)
	}

	// top border
	hline("┌", "┬", "┐", "─")

	// header row — unstyled
	fmt.Fprint(out, "│")
	for i, h := range t.Headers {
		pad := widths[i] - runeLen(h)
		fmt.Fprintf(out, " %s%s │", h, strings.Repeat(" ", pad))
	}
	fmt.Fprintln(out)

	// header / body separator
	hline("├", "┼", "┤", "─")

	// data rows — grouped by first-column value
	prevGroup := ""
	for ri, row := range t.Rows {
		firstCol := ""
		if len(row) > 0 {
			firstCol = row[0].Text
		}

		// draw a group separator whenever the first-column value changes
		// (never before the very first data row — the header separator covers that)
		if ri > 0 && firstCol != "" && firstCol != prevGroup {
			hline("├", "┼", "┤", "─")
		}

		fmt.Fprint(out, "│")
		for i, w := range widths {
			var cell TableCell
			if i < len(row) {
				cell = row[i]
			}
			plain := truncateStr(cell.Text, w)
			// suppress repeated first-column values within a group
			if i == 0 && cell.Text == prevGroup {
				plain = ""
			}
			styledCell := TableCell{Text: plain, Severity: cell.Severity}
			styled := styleCell(styledCell)
			// pad to column width using rune count (ANSI codes are invisible and
			// multi-byte chars like ✓/✗ are one visual column each)
			pad := w - runeLen(plain)
			fmt.Fprintf(out, " %s%s │", styled, strings.Repeat(" ", pad))
		}
		fmt.Fprintln(out)

		if firstCol != "" {
			prevGroup = firstCol
		}
	}

	// bottom border
	hline("└", "┴", "┘", "─")
}
