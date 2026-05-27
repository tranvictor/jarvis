package ui

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	runewidth "github.com/mattn/go-runewidth"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// BorderColor maps a Severity to the lipgloss colour used for rendering
// table / panel borders. Exposed so callers outside this package (and
// renderTable in table.go) share a single source of truth for the colour
// scheme — bumping a Severity here changes both the inline text colour
// (via Aurora in TerminalUI.Style) and the border colour consistently.
//
// The Info default ("240") matches the historical dim-grey rendering, so
// every existing Table caller looks identical until they opt-in to a
// non-zero BorderSeverity.
func BorderColor(s Severity) lipgloss.Color {
	switch s {
	case SeveritySuccess:
		return lipgloss.Color("2") // plain green, matches aurora.Green
	case SeverityWarn:
		return lipgloss.Color("3") // yellow
	case SeverityError:
		return lipgloss.Color("1") // red
	case SeverityCritical:
		return lipgloss.Color("15") // bold white
	default:
		return lipgloss.Color("240")
	}
}

// renderBoxedSection produces a rounded-bordered box around body's bytes
// using the colour matching severity, with title injected into the top
// border (e.g. "╭─ Clear Signed · ERC-7730 ─────────╮"). The result has
// no surrounding prefix — callers (TerminalUI.BoxedSection) prepend their
// indent prefix to every line before writing.
func renderBoxedSection(severity Severity, title string, body string) string {
	body = strings.TrimRight(body, "\n")
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BorderColor(severity)).
		Padding(0, 1)
	boxed := style.Render(body)
	if title != "" {
		boxed = injectBoxTitle(boxed, title, BorderColor(severity))
	}
	return boxed
}

// injectBoxTitle overwrites the top border of a lipgloss rounded box with
// "╭─ <title> ──...──╮", preserving the right-hand corner so the box stays
// aligned. We operate on the rendered (ANSI-coloured) string directly:
// stripping ANSI to measure the visual width, then re-wrapping the new
// top border with the same border colour for visual consistency.
func injectBoxTitle(boxed string, title string, color lipgloss.Color) string {
	lines := strings.SplitN(boxed, "\n", 2)
	if len(lines) == 0 {
		return boxed
	}
	top := lines[0]
	plainTop := ansi.Strip(top)
	totalWidth := runewidth.StringWidth(plainTop)
	if totalWidth < 4 {
		return boxed
	}

	// Build the replacement top border at the same visual width.
	titledChunk := "─ " + title + " "
	titledWidth := runewidth.StringWidth(titledChunk)
	// One char each for the left/right corners; the rest is filler.
	fillerWidth := totalWidth - 2 - titledWidth
	if fillerWidth < 0 {
		// Title is wider than the box: just truncate the title.
		maxTitleW := totalWidth - 2 /* corners */ - 4 /* "─ " + " " + at least one filler "─" */
		if maxTitleW < 1 {
			return boxed
		}
		title = truncateToWidth(title, maxTitleW)
		titledChunk = "─ " + title + " "
		titledWidth = runewidth.StringWidth(titledChunk)
		fillerWidth = totalWidth - 2 - titledWidth
		if fillerWidth < 0 {
			fillerWidth = 0
		}
	}
	newTop := "╭" + titledChunk + strings.Repeat("─", fillerWidth) + "╮"

	style := lipgloss.NewStyle().Foreground(color)
	colored := style.Render(newTop)

	if len(lines) == 1 {
		return colored
	}
	return colored + "\n" + lines[1]
}

func truncateToWidth(s string, maxWidth int) string {
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if rw == 0 {
			rw = 1
		}
		if w+rw > maxWidth {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String()
}

// captureBox is the io.Writer used by TerminalUI.BoxedSection so the body
// callback can target the same UI interface as the outer scope; whatever
// the body emits is buffered, framed, and written back out with the
// outer UI's indent prefix.
type captureBox struct{ buf *bytes.Buffer }

func (c *captureBox) Write(p []byte) (int, error) { return c.buf.Write(p) }

// Compile-time guard.
var _ io.Writer = (*captureBox)(nil)

// writeBoxed renders body's accumulated output as a coloured-border box
// and writes the framed lines (one per line) to out with prefix applied.
// Shared by the live and indented TerminalUI cases.
func writeBoxed(out io.Writer, prefix string, severity Severity, title string, body string) {
	boxed := renderBoxedSection(severity, title, body)
	for _, line := range strings.Split(boxed, "\n") {
		fmt.Fprintf(out, "%s%s\n", prefix, line)
	}
}
