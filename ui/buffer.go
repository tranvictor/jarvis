package ui

import (
	"bufio"
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/logrusorgru/aurora"
	runewidth "github.com/mattn/go-runewidth"
)

// NewTerminalUIWithWriter creates a TerminalUI that writes to w instead of
// os.Stdout. Useful for capturing colored output into a buffer.
func NewTerminalUIWithWriter(w io.Writer, colorsEnabled bool) *TerminalUI {
	return &TerminalUI{
		out: &blankTracker{w: w},
		in:  bufio.NewReader(strings.NewReader("")),
		au:  aurora.NewAurora(colorsEnabled),
	}
}

// LimitWidth caps the visual columns this UI may occupy, including indent.
// 0 clears the cap and falls back to the terminal width.
func (u *TerminalUI) LimitWidth(cols int) {
	if cols < 0 {
		cols = 0
	}
	u.width = cols
}

// VisibleWidth returns the terminal display width of s.
// It strips ANSI escape sequences first, then uses runewidth to correctly
// account for multi-byte characters (e.g. box-drawing chars, CJK) that occupy
// more than one byte but exactly one (or two) visual columns.
func VisibleWidth(s string) int {
	return runewidth.StringWidth(ansi.Strip(s))
}
