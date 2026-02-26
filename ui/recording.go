package ui

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Entry records a single UI method call for test assertions.
type Entry struct {
	Method string
	Value  string // the formatted string passed to the method (or input for Ask)
}

// sharedState holds the mutable state shared across a RecordingUI and all
// child UIs created via Indent(). Using a shared pointer ensures that Ask
// calls in a nested scope advance the same input cursor.
type sharedState struct {
	entries []Entry
	inputs  []string // scripted responses served in order to Ask/Confirm/Choose
	nextIdx int
	buf     *bytes.Buffer
}

// RecordingUI implements UI for tests.
//
// All output is captured in an entry log that can be inspected with
// [RecordingUI.Entries] and [RecordingUI.HasMessage]. Input is served in
// order from the scripted inputs provided to [NewRecordingUI].
//
// If a test Ask/Confirm/Choose call runs out of scripted inputs the call
// panics with a descriptive message, making test failures obvious.
//
// Shared input sequencing:
// Child UIs created via Indent() share the same input queue as their parent,
// so you can script inputs for a nested flow from the top-level RecordingUI.
type RecordingUI struct {
	shared      *sharedState
	indentLevel int
}

// NewRecordingUI creates a RecordingUI with the given scripted inputs.
// Inputs are returned by Ask/Confirm/Choose in the order they are provided.
func NewRecordingUI(scriptedInputs ...string) *RecordingUI {
	return &RecordingUI{
		shared: &sharedState{
			inputs: scriptedInputs,
			buf:    &bytes.Buffer{},
		},
	}
}

func (r *RecordingUI) record(method, value string) {
	r.shared.entries = append(r.shared.entries, Entry{
		Method: method,
		Value:  value,
	})
}

func (r *RecordingUI) nextInput(caller string) string {
	if r.shared.nextIdx >= len(r.shared.inputs) {
		panic(fmt.Sprintf(
			"RecordingUI: no scripted input left for %s (consumed %d so far)",
			caller, r.shared.nextIdx,
		))
	}
	input := r.shared.inputs[r.shared.nextIdx]
	r.shared.nextIdx++
	return input
}

// Style returns the plain text of t without any colour markup.
// RecordingUI is colour-free so tests receive clean, predictable strings.
func (r *RecordingUI) Style(t StyledText) string {
	return t.Text
}

func (r *RecordingUI) Info(format string, args ...any) {
	r.record("Info", fmt.Sprintf(format, args...))
}

func (r *RecordingUI) Success(format string, args ...any) {
	r.record("Success", fmt.Sprintf(format, args...))
}

func (r *RecordingUI) Warn(format string, args ...any) {
	r.record("Warn", fmt.Sprintf(format, args...))
}

func (r *RecordingUI) Error(format string, args ...any) {
	r.record("Error", fmt.Sprintf(format, args...))
}

func (r *RecordingUI) Critical(format string, args ...any) {
	r.record("Critical", fmt.Sprintf(format, args...))
}

func (r *RecordingUI) Section(title string) {
	r.record("Section", title)
}

func (r *RecordingUI) Interpret(value string) {
	r.record("Interpret", value)
}

// Ask returns the next scripted input. If validate is non-nil and the input
// fails validation, the test panics immediately rather than looping (since
// there is no real user to correct it — the test script is wrong).
func (r *RecordingUI) Ask(validate func(string) error) string {
	input := r.nextInput("Ask")
	r.record("Ask", input)
	if validate != nil {
		if err := validate(input); err != nil {
			panic(fmt.Sprintf(
				"RecordingUI: scripted input %q failed validation in Ask: %s",
				input, err,
			))
		}
	}
	return input
}

// Confirm returns the next scripted input interpreted as a boolean.
// Accepted values: "y", "yes" → true; "n", "no" → false; "" → defaultYes.
func (r *RecordingUI) Confirm(prompt string, defaultYes bool) bool {
	r.record("Confirm", prompt)
	input := strings.ToLower(strings.TrimSpace(r.nextInput("Confirm")))
	if input == "" {
		return defaultYes
	}
	return input == "y" || input == "yes"
}

// Choose returns the 0-based index matching the next scripted input.
// The input may be a 1-based number ("1", "2", …) or the option text itself
// (case-insensitive). The test panics if the input matches nothing.
func (r *RecordingUI) Choose(prompt string, options []string) int {
	r.record("Choose", prompt)
	input := r.nextInput("Choose")
	// try numeric 1-based index first
	if idx, err := strconv.Atoi(strings.TrimSpace(input)); err == nil {
		if idx >= 1 && idx <= len(options) {
			return idx - 1
		}
	}
	// try matching option text (case-insensitive)
	for i, opt := range options {
		if strings.EqualFold(input, opt) {
			return i
		}
	}
	panic(fmt.Sprintf(
		"RecordingUI: scripted input %q does not match any option in Choose(%q, %v)",
		input, prompt, options,
	))
}

// KeyValue records each label/value pair as a separate "KeyValue" entry so
// tests can assert on individual fields with HasMessage.
func (r *RecordingUI) KeyValue(rows [][2]string) {
	for _, row := range rows {
		r.record("KeyValue", row[0]+": "+row[1])
	}
}

// Table records each data row (not the header) as a pipe-separated "Table"
// entry so tests can assert on cell contents with HasMessage.
func (r *RecordingUI) Table(headers []string, rows [][]string) {
	r.record("Table", strings.Join(headers, " | "))
	for _, row := range rows {
		r.record("Table", strings.Join(row, " | "))
	}
}

// TableWithGroups records each group's rows as pipe-separated "Table" entries
// with a "---" separator entry between groups, mirroring the visual divider
// that TerminalUI draws.
func (r *RecordingUI) TableWithGroups(headers []string, groups [][][]string) {
	if len(headers) > 0 {
		r.record("Table", strings.Join(headers, " | "))
	}
	for gi, group := range groups {
		if gi > 0 {
			r.record("Table", "---")
		}
		for _, row := range group {
			r.record("Table", strings.Join(row, " | "))
		}
	}
}

// Spinner is a no-op in RecordingUI — no goroutines, no output.
// The returned function is also a no-op.
func (r *RecordingUI) Spinner(_ string) func() {
	return func() {}
}

// Indent returns a child RecordingUI at one deeper indent level.
// The child shares the same entry log and input queue as the parent.
func (r *RecordingUI) Indent() UI {
	return &RecordingUI{
		shared:      r.shared,
		indentLevel: r.indentLevel + 1,
	}
}

// Writer returns a writer that appends to the internal buffer.
// Indentation is not applied in RecordingUI since tests rarely need it.
func (r *RecordingUI) Writer() io.Writer {
	return r.shared.buf
}

// --- Test helpers ---

// Entries returns all recorded UI calls in order.
func (r *RecordingUI) Entries() []Entry {
	return r.shared.entries
}

// InfoMessages returns only the values recorded by Info calls.
func (r *RecordingUI) InfoMessages() []string {
	return r.methodValues("Info")
}

// ErrorMessages returns only the values recorded by Error calls.
func (r *RecordingUI) ErrorMessages() []string {
	return r.methodValues("Error")
}

// CriticalMessages returns only the values recorded by Critical calls.
func (r *RecordingUI) CriticalMessages() []string {
	return r.methodValues("Critical")
}

// HasMessage returns true if any recorded entry's value contains substr
// (case-insensitive substring match).
func (r *RecordingUI) HasMessage(substr string) bool {
	lower := strings.ToLower(substr)
	for _, e := range r.shared.entries {
		if strings.Contains(strings.ToLower(e.Value), lower) {
			return true
		}
	}
	return false
}

// Output returns everything written to Writer() as a string.
func (r *RecordingUI) Output() string {
	return r.shared.buf.String()
}

func (r *RecordingUI) methodValues(method string) []string {
	var out []string
	for _, e := range r.shared.entries {
		if e.Method == method {
			out = append(out, e.Value)
		}
	}
	return out
}
