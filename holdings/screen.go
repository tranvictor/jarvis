package holdings

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	runewidth "github.com/mattn/go-runewidth"
	"github.com/tranvictor/jarvis/ui"
	"golang.org/x/term"
)

// liveScreen paints a portfolio frame. On a terminal that can move the
// cursor it takes the alternate screen and rewrites a line by jumping to
// that row. A pipe or a dumb terminal prints the whole frame.
type liveScreen struct {
	out    io.Writer
	cursor bool
	alt    bool
	// rows and cols override the terminal size. Tests set both.
	rows, cols int
	prev       []string
	exitFrame  string
}

func newLiveScreen(out io.Writer) *liveScreen {
	return &liveScreen{out: out, cursor: cursorMotionOK()}
}

func cursorMotionOK() bool {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}
	name := strings.TrimSpace(os.Getenv("TERM"))
	return name != "" && name != "dumb"
}

func (s *liveScreen) measure() (rows, cols int) {
	if s.rows > 0 && s.cols > 0 {
		return s.rows, s.cols
	}
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w < 2 || h < 1 {
		return 0, 0
	}
	return h, w
}

func (s *liveScreen) enter() {
	if !s.cursor || s.alt {
		return
	}
	fmt.Fprint(s.out, "\033[?1049h\033[H\033[2J\033[?25l")
	s.alt = true
}

func (s *liveScreen) leave() {
	if !s.alt {
		return
	}
	last := s.exitFrame
	fmt.Fprint(s.out, "\033[?25h\033[?1049l")
	s.alt = false
	if last == "" {
		return
	}
	if !strings.HasSuffix(last, "\n") {
		last += "\n"
	}
	fmt.Fprint(s.out, last)
}

func (s *liveScreen) paint(frame string) {
	lines := frameLines(frame)
	rows, cols := s.measure()
	if !s.cursor || rows < 1 || cols < 2 {
		var b bytes.Buffer
		for _, ln := range lines {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
		_, _ = s.out.Write(b.Bytes())
		s.prev = nil
		return
	}
	if s.alt {
		s.exitFrame = frame
	}
	vis := viewport(lines, cols, rows)
	var b bytes.Buffer
	n := len(vis)
	if n > rows {
		n = rows
	}
	for i := 0; i < n; i++ {
		if i < len(s.prev) && s.prev[i] == vis[i] {
			continue
		}
		fmt.Fprintf(&b, "\033[%d;1H%s\033[K", i+1, vis[i])
	}
	clearTo := len(s.prev)
	if clearTo > rows {
		clearTo = rows
	}
	for i := n; i < clearTo; i++ {
		fmt.Fprintf(&b, "\033[%d;1H\033[K", i+1)
	}
	_, _ = s.out.Write(b.Bytes())
	s.prev = append([]string(nil), vis[:n]...)
}

func frameLines(frame string) []string {
	frame = strings.TrimRight(frame, "\n")
	if frame == "" {
		return nil
	}
	return strings.Split(frame, "\n")
}

// viewport fits the frame onto the screen. Each line stays one column
// short of the width, so the terminal never wraps it and a following
// cursor move cannot skip a row. A frame taller than the screen keeps
// the top and the last line (the live status) and says how many lines
// were left out.
func viewport(lines []string, cols, rows int) []string {
	max := cols - 1
	if max < 1 {
		max = 1
	}
	var vis []string
	for _, ln := range lines {
		vis = append(vis, wrapCols(ln, max)...)
	}
	if rows < 1 || len(vis) <= rows {
		return vis
	}
	if rows == 1 {
		return []string{truncateCols(vis[0], max)}
	}
	tail := vis[len(vis)-1]
	if rows == 2 {
		note := truncateCols(fmt.Sprintf("… %d lines hidden", len(vis)-1), max)
		return []string{note, tail}
	}
	hidden := len(vis) - (rows - 1)
	note := truncateCols(fmt.Sprintf("… %d lines hidden", hidden), max)
	head := append([]string{}, vis[:rows-2]...)
	return append(head, note, tail)
}

func wrapCols(s string, max int) []string {
	if max < 1 || visibleCols(s) <= max {
		return []string{s}
	}
	var lines []string
	var b strings.Builder
	width := 0
	open := ""
	flush := func() {
		if open != "" {
			b.WriteString("\033[0m")
		}
		lines = append(lines, b.String())
		b.Reset()
		width = 0
		if open != "" {
			b.WriteString(open)
		}
	}
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			seq, next := readCSI(s, i)
			if next == i {
				i++
				continue
			}
			b.WriteString(seq)
			if strings.HasSuffix(seq, "m") {
				if seq == "\033[0m" || seq == "\033[m" {
					open = ""
				} else {
					open = seq
				}
			}
			i = next
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runewidth.RuneWidth(r)
		if rw < 0 {
			rw = 0
		}
		if rw > max {
			if width > 0 {
				flush()
			}
			b.WriteRune(r)
			flush()
			i += size
			continue
		}
		if width+rw > max && width > 0 {
			flush()
		}
		b.WriteRune(r)
		width += rw
		i += size
	}
	if b.Len() > 0 || len(lines) == 0 {
		lines = append(lines, b.String())
	}
	return lines
}

func truncateCols(s string, max int) string {
	parts := wrapCols(s, max)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func visibleCols(s string) int {
	return ui.VisibleWidth(s)
}

func readCSI(s string, i int) (seq string, next int) {
	if i >= len(s) || s[i] != 0x1b {
		return "", i
	}
	if i+1 < len(s) && s[i+1] == '[' {
		j := i + 2
		for j < len(s) {
			c := s[j]
			j++
			if c >= 0x40 && c <= 0x7e {
				return s[i:j], j
			}
		}
		return s[i:], len(s)
	}
	if i+1 < len(s) {
		return s[i : i+2], i + 2
	}
	return s[i:], len(s)
}
