package ui

import (
	"bufio"
	"io"
	"strings"

	"github.com/logrusorgru/aurora"
)

// NewTerminalUIWithWriter creates a TerminalUI that writes to w instead of
// os.Stdout. Useful for capturing colored output into a buffer.
func NewTerminalUIWithWriter(w io.Writer, colorsEnabled bool) *TerminalUI {
	return &TerminalUI{
		out: w,
		in:  bufio.NewReader(strings.NewReader("")),
		au:  aurora.NewAurora(colorsEnabled),
	}
}

// VisibleWidth returns the display width of s, excluding ANSI escape sequences.
func VisibleWidth(s string) int {
	n := 0
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < '@' || s[j] > '~') {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
		} else {
			n++
			i++
		}
	}
	return n
}
