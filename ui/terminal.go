package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/logrusorgru/aurora"
	indent "github.com/openconfig/goyang/pkg/indent"
	"golang.org/x/term"
)

const (
	indentUnit  = "  "   // 2 spaces per indent level
	sectionWidth = 50    // total character width for Section separators
	promptPrefix = "> "  // shown on the input line before the cursor
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
}

// NewTerminalUI creates a TerminalUI that writes to os.Stdout and reads from
// os.Stdin. Colours are enabled automatically when stdout is a real terminal.
func NewTerminalUI() *TerminalUI {
	colorsEnabled := term.IsTerminal(int(os.Stdout.Fd()))
	return &TerminalUI{
		out: os.Stdout,
		in:  bufio.NewReader(os.Stdin),
		au:  aurora.NewAurora(colorsEnabled),
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
	line := strings.Repeat("=", left) + titled + strings.Repeat("=", right)
	fmt.Fprintf(u.out, "\n%s%s\n\n", u.prefix(), line)
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
	if len(rows) == 0 {
		return
	}
	maxLabel := 0
	for _, r := range rows {
		if len(r[0]) > maxLabel {
			maxLabel = len(r[0])
		}
	}
	p := u.prefix()
	for _, r := range rows {
		fmt.Fprintf(u.out, "%s%-*s  %s\n", p, maxLabel, r[0], r[1])
	}
}

// Table renders a full bordered table. Delegates to TableWithGroups.
func (u *TerminalUI) Table(headers []string, rows [][]string) {
	u.TableWithGroups(headers, [][][]string{rows})
}

// TableWithGroups renders a bordered table where each group of rows is
// separated from the next by a horizontal rule.
// Converts plain strings to TableCells and delegates to renderTable in table.go.
func (u *TerminalUI) TableWithGroups(headers []string, groups [][][]string) {
	if len(groups) == 0 {
		return
	}
	t := &Table{
		Headers: headers,
		Groups:  make([][][]TableCell, len(groups)),
	}
	for gi, group := range groups {
		t.Groups[gi] = make([][]TableCell, len(group))
		for ri, row := range group {
			t.Groups[gi][ri] = make([]TableCell, len(row))
			for ci, cell := range row {
				t.Groups[gi][ri][ci] = TC(cell)
			}
		}
	}
	renderTable(u.out, u.prefix(), t, func(cell TableCell) string { return cell.Text })
}

// PrintTable renders t as a bordered table with Aurora colour applied per cell.
// Respects the current indent level by prepending u.prefix() to every line.
func (u *TerminalUI) PrintTable(t *Table) {
	renderTable(u.out, u.prefix(), t, func(cell TableCell) string {
		return u.Style(StyledText{Text: cell.Text, Severity: cell.Severity})
	})
}

// Spinner starts an animated spinner with msg and returns a stop function.
// The stop function clears the spinner line. On non-terminal outputs the
// spinner is a no-op and only the message is printed once.
func (u *TerminalUI) Spinner(msg string) func() {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintf(u.out, "%s%s\n", u.prefix(), msg)
		return func() {}
	}
	s := spinner.New(spinner.CharSets[14], 80*time.Millisecond, spinner.WithWriter(u.out))
	s.Suffix = " " + msg
	s.Start()
	return func() {
		s.Stop()
		// briandowns/spinner clears the line with \r but no trailing \n,
		// so we emit one to ensure the next output starts on a fresh line.
		fmt.Fprintf(u.out, "\n")
	}
}

// Indent returns a child UI at one deeper indent level.
// The child shares the underlying writer and reader with the parent, so
// input sequencing and output ordering are preserved across nested scopes.
func (u *TerminalUI) Indent() UI {
	return &TerminalUI{
		indentLevel: u.indentLevel + 1,
		out:         u.out,
		in:          u.in,
		au:          u.au,
	}
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
