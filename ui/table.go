package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	runewidth "github.com/mattn/go-runewidth"
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
// Data can be specified in one of two ways (Groups takes precedence):
//
//  1. Rows (first-column auto-grouping): consecutive rows that share the same
//     first-column value form a visual group; a rule is drawn between groups.
//     Use this when the group key is embedded in the first column
//     (e.g. network name in the node list).
//
//  2. Groups (explicit grouping): each inner slice is a separate group; a
//     horizontal rule is drawn between adjacent groups.
//     Use this for structurally distinct sections (e.g. one group per log
//     event or per function call).
//
// MaxCellWidth caps any single column's visual width (0 = no cap).
// When set, cells wider than the cap are truncated with "...".
type Table struct {
	Headers      []string
	Rows         [][]TableCell   // first-column grouped; used when Groups is nil
	Groups       [][][]TableCell // explicit groups; takes precedence over Rows when non-nil
	MaxCellWidth int             // 0 = no cap
}

// AddRow appends a row to Rows (first-column auto-grouping mode).
func (t *Table) AddRow(cells ...TableCell) {
	t.Rows = append(t.Rows, cells)
}

// runeLen returns the visible terminal width of s, stripping ANSI escape codes
// first so that colour sequences don't inflate the measurement.
func runeLen(s string) int {
	return runewidth.StringWidth(ansi.Strip(s))
}

func colWidths(t *Table) []int {
	n := len(t.Headers)
	widths := make([]int, n)
	for i, h := range t.Headers {
		widths[i] = runeLen(h)
	}
	consider := func(row []TableCell) {
		for i, cell := range row {
			if i >= n {
				break
			}
			w := runeLen(cell.Text)
			if t.MaxCellWidth > 0 && w > t.MaxCellWidth {
				w = t.MaxCellWidth
			}
			if w > widths[i] {
				widths[i] = w
			}
		}
	}
	for _, row := range t.Rows {
		consider(row)
	}
	for _, group := range t.Groups {
		for _, row := range group {
			consider(row)
		}
	}
	return widths
}

// truncateStr truncates s to at most maxRunes visible code points, appending
// "..." when truncation occurs (preserves rune boundaries).
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

// renderTable is the single table rendering engine used by PrintTable,
// Table, and TableWithGroups.
//
// prefix is prepended to every output line — callers pass u.prefix() so
// that nested UI indent levels are respected.
//
// styleCell is called for each data cell (not headers) and may inject ANSI
// escape codes; padding is computed from the plain-text width so ANSI codes
// never break column alignment.
func renderTable(out io.Writer, prefix string, t *Table, styleCell func(TableCell) string) {
	if len(t.Headers) == 0 {
		return
	}
	widths := colWidths(t)
	n := len(widths)

	// Border characters are dimmed to keep them visually subordinate to content.
	bdrStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	bdr := func(s string) string { return bdrStyle.Render(s) }

	hline := func(left, mid, right string) {
		parts := make([]string, n)
		for i, w := range widths {
			parts[i] = strings.Repeat("─", w+2)
		}
		fmt.Fprintf(out, "%s%s\n", prefix, bdr(left+strings.Join(parts, mid)+right))
	}

	writeRow := func(row []TableCell, suppressFirstCol bool) {
		cells := make([]string, n)
		for i, w := range widths {
			var cell TableCell
			if i < len(row) {
				cell = row[i]
			}
			plain := cell.Text
			if t.MaxCellWidth > 0 {
				plain = truncateStr(plain, w) // w is already capped by colWidths
			}
			if suppressFirstCol && i == 0 {
				plain = ""
			}
			styled := styleCell(TableCell{Text: plain, Severity: cell.Severity})
			pad := w - runeLen(plain)
			cells[i] = " " + styled + strings.Repeat(" ", pad) + " "
		}
		fmt.Fprintf(out, "%s%s\n", prefix,
			bdr("│")+strings.Join(cells, bdr("│"))+bdr("│"))
	}

	// header row
	hline("┌", "┬", "┐")
	headerCells := make([]string, n)
	for i, h := range t.Headers {
		pad := widths[i] - runeLen(h)
		headerCells[i] = " " + h + strings.Repeat(" ", pad) + " "
	}
	fmt.Fprintf(out, "%s%s\n", prefix,
		bdr("│")+strings.Join(headerCells, bdr("│"))+bdr("│"))
	hline("├", "┼", "┤")

	if len(t.Groups) > 0 {
		// explicit grouping: each slice in Groups is a separate visual section
		for gi, group := range t.Groups {
			if gi > 0 {
				hline("├", "┼", "┤")
			}
			for _, row := range group {
				writeRow(row, false)
			}
		}
	} else {
		// first-column auto-grouping: rows with the same first-column value
		// are shown together; a rule separates each change in that value.
		prevGroup := ""
		for ri, row := range t.Rows {
			firstCol := ""
			if len(row) > 0 {
				firstCol = row[0].Text
			}
			if ri > 0 && firstCol != "" && firstCol != prevGroup {
				hline("├", "┼", "┤")
			}
			writeRow(row, firstCol != "" && firstCol == prevGroup)
			if firstCol != "" {
				prevGroup = firstCol
			}
		}
	}

	hline("└", "┴", "┘")
}
