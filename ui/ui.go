package ui

import (
	"encoding/json"
	"io"
)

// Severity classifies the visual weight of a piece of inline text, mirroring
// the five output methods on UI. The print layer maps each value to the
// corresponding terminal style; data consumers (JSON, tests) see plain text.
type Severity uint8

const (
	SeverityInfo     Severity = iota // plain — no colour emphasis
	SeveritySuccess                  // green  — known / positive
	SeverityWarn                     // yellow — uncertain / needs attention
	SeverityError                    // red    — unknown / negative
	SeverityCritical                 // bold   — must-review before action
)

// StyledText pairs a plain string with a Severity annotation.
//
// JSON serialization: the struct marshals as just the plain Text string so
// consumers receive clean output with no ANSI codes and no extra structure.
//
// Terminal rendering: pass the value to [UI.Style] to obtain the
// appropriately coloured string for embedding in a format call:
//
//	u.Info("From: %s", u.Style(d.From))
type StyledText struct {
	Text     string
	Severity Severity
}

// MarshalJSON serializes StyledText as a plain JSON string (just Text).
func (s StyledText) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Text)
}

// UI provides all terminal interaction for Jarvis commands.
//
// It abstracts output, user prompts, and indentation so that:
//   - Production code uses TerminalUI (writes to os.Stdout, reads from os.Stdin)
//   - Tests use RecordingUI (captures all output, serves scripted inputs)
//
// Indentation / nesting
//
// Use [UI.Indent] to get a child UI at one deeper indent level. Pass the child
// into functions that need nested prompts (e.g. entering each array element
// inside a parameter). The child shares the same underlying writer and reader,
// so input sequencing is preserved across scopes.
//
// Typical interactive parameter flow:
//
//	ui.Info("1. to (address)")   // label on its own line
//	val := ui.Ask(nil)           // "> " prompt where user types
//	ui.Interpret("0xd8dA... (Vitalik Buterin)") // "→ ..." shows what Jarvis understood
type UI interface {
	// --- Output ---

	// Style returns the text from t coloured according to its Severity.
	// Use this to embed a styled value inside a larger Info/Critical/... line:
	//
	//	u.Info("From: %s ===> %s", u.Style(d.From), u.Style(d.To))
	//
	// When colours are disabled (e.g. piped output, RecordingUI) the plain
	// text is returned unchanged.
	Style(t StyledText) string

	// Info writes a neutral status line (no prefix, no color).
	Info(format string, args ...any)

	// Success writes a positive outcome in green.
	Success(format string, args ...any)

	// Warn writes a non-fatal warning in yellow.
	Warn(format string, args ...any)

	// Error writes a failure in red.
	// This does NOT exit or return an error — callers decide what to do next.
	Error(format string, args ...any)

	// Critical writes data the user must review before taking an irreversible
	// action — anything related to a transaction they are about to sign, or
	// proof of a transaction they just broadcast.
	//
	// In the current terminal implementation it renders as bold text so it
	// stands out from plain Info output. In a future split-pane TUI
	// implementation, Critical calls will be routed to a dedicated "summary"
	// panel on the right while the activity log continues on the left.
	Critical(format string, args ...any)

	// Section writes a visual separator centred around a title.
	// Example: "===== Confirm tx data before signing ====="
	Section(title string)

	// KeyValue renders an aligned 2-column block — label on the left,
	// value on the right — with all values left-aligned to the same column.
	// Use for compact metadata like Status/From/To/Value or gas details.
	KeyValue(rows [][2]string)

	// Table renders a full bordered table with a header row followed by data
	// rows. Use when there are 3+ columns or the data is inherently tabular
	// (e.g. a decoded parameter list).
	Table(headers []string, rows [][]string)

	// TableWithGroups renders a bordered table where each group of rows is
	// visually separated from the next by a horizontal divider line. Use when
	// rows belong to distinct logical groups (e.g. one group per event log).
	TableWithGroups(headers []string, groups [][][]string)

	// Spinner starts an animated spinner with the given message and returns a
	// stop function. Call the stop function (or defer it) to clear the spinner
	// once the work is done:
	//
	//   stop := u.Spinner("Fetching transaction...")
	//   defer stop()
	//
	// In RecordingUI and non-terminal contexts the stop function is a no-op.
	Spinner(msg string) func()

	// Interpret writes what Jarvis understood from the user's last input.
	// Always shown immediately after Ask, indented and prefixed with "→".
	// Example: "  → 8,800,000,000,000,000,000 (8.8 KNC)"
	Interpret(value string)

	// --- Input ---

	// Ask displays a "> " prompt at the current indent level and reads a line.
	// It loops until validate returns nil. Pass nil to accept any input.
	// The caller is responsible for printing a label line before calling Ask.
	Ask(validate func(string) error) string

	// Confirm asks a yes/no question and returns the boolean answer.
	// It prints the prompt text followed by [Y/n] or [y/N], then a "> " cursor.
	Confirm(prompt string, defaultYes bool) bool

	// Choose prints a numbered list of options, prompts for a selection,
	// and returns the 0-based index of the chosen option.
	Choose(prompt string, options []string) int

	// --- Nesting ---

	// Indent returns a child UI with indent level increased by one,
	// sharing the same underlying writer and reader as the parent.
	Indent() UI

	// Writer returns an io.Writer that prepends the current indentation
	// to every line. Use this when calling functions that take io.Writer
	// directly (e.g. common.PrintVerboseParamResultToWriter).
	Writer() io.Writer
}
