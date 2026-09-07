package ui

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/logrusorgru/aurora"
	indent "github.com/openconfig/goyang/pkg/indent"
	"golang.org/x/term"
)

const (
	indentUnit      = "  " // 2 spaces per indent level
	sectionWidth    = 50   // total character width for Section separators
	promptPrefix    = "> " // shown on the input line before the cursor
	interpretPrefix = "→ " // shown after Ask to display what Jarvis understood
)

// TerminalUI is the production UI implementation.
// It writes coloured output to os.Stdout and reads input from os.Stdin.
// Indentation is tracked as a level count; each level adds two spaces.
type TerminalUI struct {
	indentLevel int
	out         io.Writer
	in          *bufio.Reader
	au          aurora.Aurora
	// tty is true only when out is an interactive terminal. It gates cursor
	// movement and animation; colours are gated separately through au so a
	// caller can force colours into a buffer without enabling animation.
	tty bool
}

// NewTerminalUI creates a TerminalUI that writes to os.Stdout and reads from
// os.Stdin. Colours are enabled automatically when stdout is a real terminal.
func NewTerminalUI() *TerminalUI {
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	return &TerminalUI{
		out: &blankTracker{w: os.Stdout},
		in:  bufio.NewReader(os.Stdin),
		au:  aurora.NewAurora(isTTY),
		tty: isTTY,
	}
}

// blankTracker remembers whether the last bytes written ended with an empty
// line so headings that want breathing room above them can avoid stacking a
// second blank line on top of one that is already there.
type blankTracker struct {
	w         io.Writer
	endsNL    bool // last write ended with '\n'
	lastBlank bool // last write ended with an empty line
}

func (b *blankTracker) Write(p []byte) (int, error) {
	if len(p) > 0 {
		switch {
		case bytes.HasSuffix(p, []byte("\n\n")):
			b.lastBlank = true
		case bytes.Equal(p, []byte("\n")):
			b.lastBlank = b.endsNL
		default:
			b.lastBlank = false
		}
		b.endsNL = p[len(p)-1] == '\n'
	}
	return b.w.Write(p)
}

// endsWithBlankLine reports whether the last thing written to u.out was an
// empty line. Always false for writers that are not tracked.
func (u *TerminalUI) endsWithBlankLine() bool {
	bt, ok := u.out.(*blankTracker)
	return ok && bt.lastBlank
}

func (u *TerminalUI) child(indentLevel int, out io.Writer, tty bool) *TerminalUI {
	return &TerminalUI{
		indentLevel: indentLevel,
		out:         out,
		in:          u.in,
		au:          u.au,
		tty:         tty,
	}
}

func (u *TerminalUI) prefix() string {
	return strings.Repeat(indentUnit, u.indentLevel)
}

// writeLine writes a single line to the output with the current indent prefix.
func (u *TerminalUI) writeLine(line string) {
	fmt.Fprintf(u.out, "%s%s\n", u.prefix(), line)
}

func (u *TerminalUI) Style(t StyledText) string {
	switch t.Severity {
	case SeveritySuccess:
		return u.au.Green(t.Text).String()
	case SeverityWarn:
		return u.au.Yellow(t.Text).String()
	case SeverityError:
		return u.au.Red(t.Text).String()
	case SeverityCritical:
		return u.au.Bold(t.Text).String()
	case SeverityMuted:
		return u.au.Faint(t.Text).String()
	default: // SeverityInfo
		return t.Text
	}
}

func (u *TerminalUI) Info(format string, args ...any) {
	u.writeLine(fmt.Sprintf(format, args...))
}

func (u *TerminalUI) Success(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	u.writeLine(u.au.Green(msg).String())
}

func (u *TerminalUI) Warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	u.writeLine(u.au.Yellow(msg).String())
}

func (u *TerminalUI) Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	u.writeLine(u.au.Red(msg).String())
}

func (u *TerminalUI) Critical(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	u.writeLine(u.au.Bold(msg).String())
}

// Section prints a separator line centred around the title, surrounded by
// blank lines so sections are visually distinct in long output.
//
// Example output:
//
//	===== Confirm tx data before signing =====
func (u *TerminalUI) Section(title string) {
	titled := " " + title + " "
	bars := sectionWidth - len(titled)
	if bars < 6 {
		bars = 6
	}
	left := bars / 2
	right := bars - left
	// The title carries the weight; the rule is there to be skimmed past.
	line := u.au.Faint(strings.Repeat("=", left)).String() +
		u.au.Bold(titled).String() +
		u.au.Faint(strings.Repeat("=", right)).String()
	fmt.Fprintf(u.out, "\n%s%s\n\n", u.prefix(), line)
}

// Subsection prints a bold heading after a blank line.
func (u *TerminalUI) Subsection(title string) {
	if !u.endsWithBlankLine() {
		fmt.Fprint(u.out, "\n")
	}
	fmt.Fprintf(u.out, "%s%s\n", u.prefix(), u.au.Bold(title).String())
}

// RewriteLastLine moves the cursor up one line, clears it and writes line
// there. Off-TTY it degrades to a plain writeLine.
func (u *TerminalUI) RewriteLastLine(line string) {
	if u.tty {
		fmt.Fprint(u.out, "\x1b[1A\x1b[2K")
	}
	u.writeLine(line)
}

// BoxedSection renders body inside a rounded coloured-border box.
//
// We capture body's output into an in-memory buffer by spawning a
// detached TerminalUI that shares the input reader (so any prompts
// still work) but writes to the buffer instead of os.Stdout. The
// captured text is then framed by lipgloss and emitted to the outer
// writer with this UI's indent prefix preserved.
func (u *TerminalUI) BoxedSection(severity Severity, title string, body func(UI)) {
	var buf bytes.Buffer
	// The buffer is not a terminal: no cursor movement or animation inside.
	body(u.child(0, &buf, false))
	writeBoxed(u.out, u.prefix(), severity, title, buf.String())
}

// Interpret shows what Jarvis understood from the user's last input.
// It is always shown indented one extra level and prefixed with "→" so it is
// visually distinct from both the prompt label and the raw input line.
func (u *TerminalUI) Interpret(value string) {
	fmt.Fprintf(u.out, "%s%s%s%s\n",
		u.prefix(),
		indentUnit,
		interpretPrefix,
		u.au.Cyan(value).String(),
	)
}

// Ask prints a "> " prompt at the current indent and reads a line from stdin.
// It repeats until validate returns nil. A nil validator accepts everything.
// Validation errors are shown via the Error style ("Jarvis: <msg>").
func (u *TerminalUI) Ask(validate func(string) error) string {
	for {
		fmt.Fprintf(u.out, "%s%s", u.prefix(), promptPrefix)
		text, _ := u.in.ReadString('\n')
		input := strings.TrimRight(text, "\r\n")
		if validate == nil {
			return input
		}
		if err := validate(input); err == nil {
			return input
		} else {
			u.writeLine(u.au.Red(err.Error()).String())
		}
	}
}

// Confirm prints a yes/no question followed by a "> " prompt and returns the
// user's answer. An empty response accepts the default.
func (u *TerminalUI) Confirm(prompt string, defaultYes bool) bool {
	options := "[Y/n]"
	if !defaultYes {
		options = "[y/N]"
	}
	u.Info("%s %s", prompt, options)
	input := strings.ToLower(strings.TrimSpace(u.Ask(func(s string) error {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || s == "y" || s == "n" {
			return nil
		}
		return fmt.Errorf("please enter y or n")
	})))
	if input == "" {
		return defaultYes
	}
	return input == "y"
}

// Choose prints a numbered list of options, then prompts for an index.
// It returns the 0-based index of the chosen option.
func (u *TerminalUI) Choose(prompt string, options []string) int {
	for i, opt := range options {
		u.Info("%d. %s", i+1, opt)
	}
	u.Info("%s [1-%d]", prompt, len(options))
	input := u.Ask(func(s string) error {
		idx, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || idx < 1 || idx > len(options) {
			return fmt.Errorf("please enter a number between 1 and %d", len(options))
		}
		return nil
	})
	idx, _ := strconv.Atoi(strings.TrimSpace(input))
	return idx - 1
}

// KeyValue renders an aligned 2-column block.
// The label column is right-padded to the width of the longest label so all
// values line up, making metadata blocks easy to scan at a glance.
func (u *TerminalUI) KeyValue(rows [][2]string) {
	cells := make([][2]TableCell, len(rows))
	for i, r := range rows {
		cells[i] = [2]TableCell{TC(r[0]), TC(r[1])}
	}
	u.KeyValueCells(cells)
}

// KeyValueCells renders an aligned 2-column block with per-cell colour.
// Padding is computed on visible width so styled labels still line up.
func (u *TerminalUI) KeyValueCells(rows [][2]TableCell) {
	if len(rows) == 0 {
		return
	}
	maxLabel := 0
	for _, r := range rows {
		if w := runeLen(r[0].Text); w > maxLabel {
			maxLabel = w
		}
	}
	p := u.prefix()
	for _, r := range rows {
		label := u.Style(StyledText{Text: r[0].Text, Severity: r[0].Severity})
		pad := strings.Repeat(" ", maxLabel-runeLen(r[0].Text))
		value := u.Style(StyledText{Text: r[1].Text, Severity: r[1].Severity})
		fmt.Fprintf(u.out, "%s%s%s  %s\n", p, label, pad, value)
	}
}

// Table renders a full bordered table of plain strings as one ungrouped
// block. Converts the cells to TableCells and delegates to renderTable.
func (u *TerminalUI) Table(headers []string, rows [][]string) {
	group := make([][]TableCell, len(rows))
	for ri, row := range rows {
		group[ri] = make([]TableCell, len(row))
		for ci, cell := range row {
			group[ri][ci] = TC(cell)
		}
	}
	t := &Table{Headers: headers, Groups: [][][]TableCell{group}}
	renderTable(u.out, u.prefix(), t, func(cell TableCell) string { return cell.Text })
}

// PrintTable renders t as a bordered table with Aurora colour applied per cell.
// Respects the current indent level by prepending u.prefix() to every line.
func (u *TerminalUI) PrintTable(t *Table) {
	renderTable(u.out, u.prefix(), t, func(cell TableCell) string {
		return u.Style(StyledText{Text: cell.Text, Severity: cell.Severity})
	})
}

// Spinner starts a live status line. On a terminal it animates and appends
// the elapsed time (m:ss), refreshing once a second; otherwise each distinct
// message is printed once as a plain line so piped output stays readable.
func (u *TerminalUI) Spinner(msg string) Progress {
	p := &terminalProgress{u: u, start: time.Now(), msg: msg}
	if !u.tty {
		u.writeLine(msg)
		return p
	}
	p.s = spinner.New(spinner.CharSets[14], 80*time.Millisecond, spinner.WithWriter(u.out))
	p.s.Prefix = u.prefix()
	p.s.Suffix = p.suffix()
	p.s.Start()
	p.done = make(chan struct{})
	go p.tick()
	return p
}

type terminalProgress struct {
	u     *TerminalUI
	s     *spinner.Spinner // nil off-TTY
	start time.Time
	mu    sync.Mutex
	msg   string
	done  chan struct{}
	once  sync.Once
}

func formatElapsed(d time.Duration) string {
	secs := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

func (p *terminalProgress) suffix() string {
	return " " + p.msg + "  " + p.u.au.Faint(formatElapsed(time.Since(p.start))).String()
}

func (p *terminalProgress) refresh() {
	p.s.Lock()
	p.s.Suffix = p.suffix()
	p.s.Unlock()
}

func (p *terminalProgress) tick() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-t.C:
			p.mu.Lock()
			p.refresh()
			p.mu.Unlock()
		}
	}
}

func (p *terminalProgress) Update(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if msg == p.msg {
		return
	}
	p.msg = msg
	if p.s == nil {
		p.u.writeLine(msg)
		return
	}
	p.refresh()
}

func (p *terminalProgress) Elapsed() time.Duration {
	return time.Since(p.start)
}

func (p *terminalProgress) Stop(final StyledText) {
	p.once.Do(func() {
		if p.s != nil {
			close(p.done)
			// briandowns/spinner clears the line with \r but no trailing \n,
			// so the final line (or the next output) starts on the same row.
			p.s.Stop()
		}
		if final.Text != "" {
			p.u.writeLine(p.u.Style(final))
		}
	})
}

// Indent returns a child UI at one deeper indent level.
// The child shares the underlying writer and reader with the parent, so
// input sequencing and output ordering are preserved across nested scopes.
func (u *TerminalUI) Indent() UI {
	return u.child(u.indentLevel+1, u.out, u.tty)
}

// Writer returns an io.Writer that automatically prepends the current
// indentation prefix to every line written to it. This lets you pass the
// UI's output context into functions that accept a plain io.Writer.
func (u *TerminalUI) Writer() io.Writer {
	if u.indentLevel == 0 {
		return u.out
	}
	return indent.NewWriter(u.out, u.prefix())
}
