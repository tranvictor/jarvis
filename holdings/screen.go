package holdings

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tranvictor/jarvis/ui"
	"golang.org/x/term"
)

// liveScreen paints a portfolio frame. On a terminal that can move the
// cursor, later paints rewrite only the lines that changed and leave the
// rest where it is. A pipe, a dumb terminal, or a line wider than the
// screen falls back to printing the whole frame.
type liveScreen struct {
	out    io.Writer
	cursor bool
	prev   []string
	placed bool
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

func (s *liveScreen) paint(frame string, cols int) {
	lines := frameLines(frame)
	fit := s.cursor && linesFit(lines, cols)
	if s.cursor && s.placed && fit {
		s.writeDiff(lines)
		s.prev = lines
		return
	}
	var b bytes.Buffer
	// The previous frame was anchored in place. A full reprint has to
	// clear it, or the old rows stay on screen under the new copy.
	if s.cursor && s.placed {
		b.WriteString("\033[H\033[2J")
	}
	for _, ln := range lines {
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	_, _ = s.out.Write(b.Bytes())
	s.prev = lines
	s.placed = fit
}

func (s *liveScreen) writeDiff(lines []string) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "\033[%dA", len(s.prev))
	for i := 0; i < len(lines); i++ {
		if i >= len(s.prev) || s.prev[i] != lines[i] {
			b.WriteByte('\r')
			b.WriteString(lines[i])
			b.WriteString("\033[K")
		}
		b.WriteByte('\n')
	}
	extra := len(s.prev) - len(lines)
	for i := 0; i < extra; i++ {
		b.WriteString("\r\033[K\n")
	}
	if extra > 0 {
		fmt.Fprintf(&b, "\033[%dA", extra)
	}
	_, _ = s.out.Write(b.Bytes())
}

func frameLines(frame string) []string {
	frame = strings.TrimRight(frame, "\n")
	if frame == "" {
		return nil
	}
	return strings.Split(frame, "\n")
}

func linesFit(lines []string, cols int) bool {
	if cols <= 0 {
		return false
	}
	for _, ln := range lines {
		if ui.VisibleWidth(ln) > cols {
			return false
		}
	}
	return true
}
