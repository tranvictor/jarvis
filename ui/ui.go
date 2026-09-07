package ui

import (
	"encoding/json"
	"io"
	"time"
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
	SeverityMuted                    // faint  — metadata that should recede (types, indices, hints)
)

// Progress is a live status line started by [UI.Spinner]. On a terminal it is
// an animated spinner with the elapsed time appended; elsewhere it degrades to
// one plain line per distinct message so logs stay readable.
type Progress interface {
	// Update replaces the message shown next to the spinner.
	Update(msg string)
	// Elapsed returns the time since the spinner was started.
	Elapsed() time.Duration
	// Stop clears the spinner and prints final as the line that stays in the
	// transcript. Pass an empty Text to print nothing.
	Stop(final StyledText)
}

// TreeBranch returns the glyph that introduces a nested item: "└─ " for the
// last sibling, "├─ " otherwise.
func TreeBranch(last bool) string {
	if last {
		return "└─ "
	}
	return "├─ "
}

// TreeGuide returns the continuation prefix placed under a TreeBranch for the
// lines that belong to the same item: "   " after a last sibling, "│  "
// otherwise, so vertical guides line up with the branch glyphs.
func TreeGuide(last bool) string {
	if last {
		return "   "
	}
	return "│  "
}

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

	// Subsection writes a bold heading preceded by a blank line, with no
	// rule. Use it for the blocks inside a Section (e.g. "Transfers",
	// "Events (4)") so they stand out without adding visual weight.
	Subsection(title string)

	// RewriteLastLine replaces the previous line of output with line. On a
	// terminal this moves the cursor up and clears the line; elsewhere it
	// simply writes line so transcripts and tests stay linear.
	RewriteLastLine(line string)

	// BoxedSection renders the output emitted by body inside a rounded
	// rectangular box whose border colour reflects severity. The optional
	// title is injected into the top border ("╭─ <title> ─...─╮") so the
	// box self-labels.
	//
	// Use it to highlight curated content the user should focus on — for
	// instance the ERC-7730 clear-signed intent that sits above the raw
	// ABI-decoded view in PromptTxConfirmation.
	//
	// body receives a child UI that delegates its output through this
	// renderer; calls like Info / Critical / Table / KeyValue inside body
	// behave normally but are captured, framed, and emitted as a single
	// block. The child shares input state (Ask, Confirm, …) with the
	// parent so interactive prompts still work if needed.
	BoxedSection(severity Severity, title string, body func(UI))

	// KeyValue renders an aligned 2-column block — label on the left,
	// value on the right — with all values left-aligned to the same column.
	// Use for compact metadata like Status/From/To/Value or gas details.
	KeyValue(rows [][2]string)

	// KeyValueCells is KeyValue with per-cell severity so labels can recede
	// (SeverityMuted) while values carry colour.
	KeyValueCells(rows [][2]TableCell)

	// Table renders a full bordered table with a header row followed by data
	// rows. Use when there are 3+ columns or the data is inherently tabular
	// (e.g. a decoded parameter list).
	Table(headers []string, rows [][]string)

	// PrintTable renders a bordered Table to the output, applying colour to
	// each cell according to its Severity. Rows that share the same first-column
	// value are visually grouped with a horizontal rule between groups; set
	// Table.Groups for explicit grouping. Use this instead of Table when cells
	// need per-cell colour (e.g. node status) or grouping.
	PrintTable(t *Table)

	// Spinner starts a live status line with the given message and returns a
	// Progress handle to update it and eventually stop it:
	//
	//   p := u.Spinner("Fetching transaction...")
	//   p.Update("Decoding calldata...")
	//   p.Stop(StyledText{Text: "✓ done", Severity: SeveritySuccess})
	//
	// On a terminal this animates and shows elapsed time; on non-terminal
	// outputs each distinct message is printed once as a plain line.
	Spinner(msg string) Progress

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
	// directly.
	Writer() io.Writer
}
