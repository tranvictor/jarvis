package ui

import "io"

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
