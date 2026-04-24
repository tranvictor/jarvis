package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	runewidth "github.com/mattn/go-runewidth"
	"golang.org/x/term"
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
// When set, cells wider than the cap are wrapped across multiple
// physical lines so the full content remains visible without blowing
// the table out to terminal-breaking widths.
//
// Even when MaxCellWidth is 0, renderTable auto-fits the table to the
// current terminal width (shrinking the widest columns first) and
// wraps any cell that exceeds its computed column width.
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
	// Infer column count from the data when no headers are supplied.
	n := len(t.Headers)
	for _, row := range t.Rows {
		if len(row) > n {
			n = len(row)
		}
	}
	for _, group := range t.Groups {
		for _, row := range group {
			if len(row) > n {
				n = len(row)
			}
		}
	}
	if n == 0 {
		return nil
	}

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

	// Fit-to-terminal: if the table as-measured would overflow the
	// current terminal, shrink the widest column(s) until the whole
	// row fits. Otherwise very long hex calldata blows the table out
	// to several thousand columns wide and ruins readability. The
	// rendering path below will wrap any cell whose content exceeds
	// its column width, so no information is lost.
	shrinkToTerminal(widths)
	return widths
}

// minWrapWidth is the smallest column width we'll ever shrink to when
// auto-fitting to the terminal. Narrower than this and hex strings
// become a scroll of 10-char fragments with more border than content.
const minWrapWidth = 20

// shrinkToTerminal adjusts widths in place so that the total rendered
// row width fits within the current terminal columns. It preferentially
// shrinks the widest column, repeating until the budget is satisfied
// or every column is at minWrapWidth (in which case the table will
// still overflow, but by as little as possible).
func shrinkToTerminal(widths []int) {
	termCols := detectTerminalWidth()
	if termCols <= 0 {
		return
	}
	// Each column renders as " content " plus one separator glyph; a
	// final glyph caps the right side. So the total frame overhead is
	// 2*n (paddings) + n + 1 (separators including the outer rails).
	n := len(widths)
	const safetyMargin = 1 // avoid writing into the very last column
	overhead := 3*n + 1 + safetyMargin
	budget := termCols - overhead
	if budget <= 0 {
		return
	}
	total := 0
	for _, w := range widths {
		total += w
	}
	for total > budget {
		// Find the widest column that's still wider than the floor.
		maxIdx, maxW := -1, -1
		for i, w := range widths {
			if w > maxW && w > minWrapWidth {
				maxIdx, maxW = i, w
			}
		}
		if maxIdx < 0 {
			return // every column already at or below the floor
		}
		widths[maxIdx]--
		total--
	}
}

// detectTerminalWidth returns os.Stdout's current column count, or 0
// if we can't determine it (not a TTY, error, etc.) — callers treat 0
// as "no cap".
func detectTerminalWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 0
}

// wrapCell splits s into visual lines of at most maxWidth columns each,
// preserving rune boundaries. Input line breaks in s are respected: each
// source line is wrapped independently so explicit newlines stay aligned
// with the original content.
//
// Wrapping is char-based (no word boundaries) because cell content in
// jarvis is usually long hex strings with no natural break points; for
// human-readable text this still looks acceptable because most such
// content is short enough not to wrap at all.
func wrapCell(s string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{s}
	}
	if s == "" {
		return []string{""}
	}
	var lines []string
	for _, src := range strings.Split(s, "\n") {
		if src == "" {
			lines = append(lines, "")
			continue
		}
		runes := []rune(src)
		start := 0
		for start < len(runes) {
			end := start
			width := 0
			for end < len(runes) {
				rw := runewidth.RuneWidth(runes[end])
				if rw == 0 {
					rw = 1
				}
				if width+rw > maxWidth {
					break
				}
				width += rw
				end++
			}
			if end == start {
				// Single rune wider than maxWidth — take it anyway
				// so we make forward progress.
				end = start + 1
			}
			lines = append(lines, string(runes[start:end]))
			start = end
		}
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
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
	widths := colWidths(t)
	n := len(widths)
	if n == 0 {
		return
	}

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

	// writeRow renders a logical table row, wrapping any cell whose
	// content exceeds its column width onto additional physical lines.
	// Cells whose content fits stay on the first physical line only;
	// their continuation lines are rendered as blank padding so the
	// bordered layout stays aligned.
	writeRow := func(row []TableCell, suppressFirstCol bool) {
		// Precompute the wrapped lines and severity for each column.
		type wrapped struct {
			lines    []string
			severity Severity
		}
		cols := make([]wrapped, n)
		maxLines := 1
		for i, w := range widths {
			var cell TableCell
			if i < len(row) {
				cell = row[i]
			}
			plain := cell.Text
			if suppressFirstCol && i == 0 {
				plain = ""
			}
			lines := wrapCell(plain, w)
			cols[i] = wrapped{lines: lines, severity: cell.Severity}
			if len(lines) > maxLines {
				maxLines = len(lines)
			}
		}
		for li := 0; li < maxLines; li++ {
			cells := make([]string, n)
			for i, w := range widths {
				plain := ""
				if li < len(cols[i].lines) {
					plain = cols[i].lines[li]
				}
				styled := styleCell(TableCell{Text: plain, Severity: cols[i].severity})
				pad := w - runeLen(plain)
				if pad < 0 {
					pad = 0
				}
				cells[i] = " " + styled + strings.Repeat(" ", pad) + " "
			}
			fmt.Fprintf(out, "%s%s\n", prefix,
				bdr("│")+strings.Join(cells, bdr("│"))+bdr("│"))
		}
	}

	hline("┌", "┬", "┐")
	if len(t.Headers) > 0 {
		// Headers wrap the same way data cells do so auto-shrinking
		// to terminal width doesn't corrupt alignment when a header
		// is longer than its (shrunken) column.
		headerLines := make([][]string, n)
		maxHdrLines := 1
		for i, w := range widths {
			var h string
			if i < len(t.Headers) {
				h = t.Headers[i]
			}
			headerLines[i] = wrapCell(h, w)
			if len(headerLines[i]) > maxHdrLines {
				maxHdrLines = len(headerLines[i])
			}
		}
		for li := 0; li < maxHdrLines; li++ {
			cells := make([]string, n)
			for i, w := range widths {
				h := ""
				if li < len(headerLines[i]) {
					h = headerLines[i][li]
				}
				pad := w - runeLen(h)
				if pad < 0 {
					pad = 0
				}
				cells[i] = " " + h + strings.Repeat(" ", pad) + " "
			}
			fmt.Fprintf(out, "%s%s\n", prefix,
				bdr("│")+strings.Join(cells, bdr("│"))+bdr("│"))
		}
		hline("├", "┼", "┤")
	}

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
