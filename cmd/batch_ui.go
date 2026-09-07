package cmd

// Shared rendering for batch commands (`jarvis msig bapprove`): the plan
// printed before any work starts, per-item banners and one-line results
// with a running tally, the closing summary table and the exit code.

import (
	"fmt"
	"os"
	"strings"

	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/ui"
)

// exitFunc is os.Exit unless a test swaps it out.
var exitFunc = os.Exit

// batchContinueOnError (--continue-on-error) keeps a batch going after a
// failed item without asking. batchConfirmOnce (--confirm-once) shows every
// Safe signing card up front and asks once before signing them all.
var (
	batchContinueOnError bool
	batchConfirmOnce     bool
)

// batchTally counts outcomes as items complete.
type batchTally struct {
	total, ok, skipped, failed int
}

func (t *batchTally) add(status string) {
	switch status {
	case "approved", "executed", "broadcasted":
		t.ok++
	case "skipped":
		t.skipped++
	case "failed":
		t.failed++
	}
}

func (t batchTally) done() int { return t.ok + t.skipped + t.failed }

// String renders "2 ok · 1 skipped · 0 failed · 3 left", dropping zero
// counters except ok so the line stays short.
func (t batchTally) String() string {
	parts := []string{fmt.Sprintf("%d ok", t.ok)}
	if t.skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", t.skipped))
	}
	if t.failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", t.failed))
	}
	if left := t.total - t.done(); left > 0 {
		parts = append(parts, fmt.Sprintf("%d left", left))
	}
	return strings.Join(parts, " · ")
}

// printBatchPlan lists every item before the first prompt so the user knows
// how many confirmations are coming and can abort early.
func printBatchPlan(title string, items []string) {
	appUI.Section(fmt.Sprintf("%s: %d transaction(s)", title, len(items)))
	for i, it := range items {
		appUI.Info("%s %s", appUI.Style(ui.StyledText{Text: fmt.Sprintf("%d.", i+1), Severity: ui.SeverityMuted}), it)
	}
}

// printBatchBanner opens one item: "[2/5] Safe  <ref>".
func printBatchBanner(i, total int, kind, label string) {
	appUI.Info("")
	appUI.Info("%s %s  %s",
		appUI.Style(ui.StyledText{Text: fmt.Sprintf("[%d/%d]", i, total), Severity: ui.SeverityCritical}),
		appUI.Style(ui.StyledText{Text: kind, Severity: ui.SeverityCritical}),
		label,
	)
}

// printBatchItemResult closes one item with a single line whose colour
// carries the outcome, followed by the running tally.
func printBatchItemResult(status, detail string, tally batchTally) {
	var st ui.StyledText
	switch status {
	case "approved", "executed", "broadcasted":
		st = ui.StyledText{Text: "✓ " + status, Severity: ui.SeveritySuccess}
	case "skipped":
		st = ui.StyledText{Text: "⊘ skipped", Severity: ui.SeverityWarn}
	default:
		st = ui.StyledText{Text: "✗ failed", Severity: ui.SeverityError}
	}
	line := appUI.Style(st)
	if detail != "" {
		line += "  " + detail
	}
	line += "   " + appUI.Style(ui.StyledText{Text: "(" + tally.String() + ")", Severity: ui.SeverityMuted})
	appUI.Info("%s", line)
}

// continueBatchAfterFailure asks whether to keep going once an item failed.
// --yes and --continue-on-error never stop; an empty answer continues.
func continueBatchAfterFailure(left int) bool {
	if config.YesToAllPrompt || batchContinueOnError || left <= 0 {
		return true
	}
	return appUI.Confirm(fmt.Sprintf("Continue with the remaining %d transaction(s)?", left), true)
}

// resultCell colours a status word for the summary table.
func resultCell(status string) ui.TableCell {
	switch status {
	case "approved", "executed", "broadcasted":
		return ui.TCS(status, ui.SeveritySuccess)
	case "skipped":
		return ui.TCS(status, ui.SeverityWarn)
	default:
		return ui.TCS(status, ui.SeverityError)
	}
}

// printBatchSummaryTable prints the closing table and the totals line.
func printBatchSummaryTable(rows [][]ui.TableCell, tally batchTally) {
	appUI.Section("Batch summary")
	appUI.PrintTable(&ui.Table{
		Headers: []string{"#", "Kind", "Network", "Target", "Result", "Detail"},
		Rows:    rows,
	})
	appUI.Info("")
	appUI.Info("%d transaction(s): %s", tally.total, strings.TrimSuffix(tally.String(), " · 0 left"))
}

// withIndentedUI runs fn with appUI one level deeper so an item's output is
// visually nested under its banner. The cmd package addresses the UI through
// the package variable, hence the swap.
func withIndentedUI(fn func()) {
	prev := appUI
	appUI = appUI.Indent()
	defer func() { appUI = prev }()
	fn()
}

// exitForBatch ends the process with status 1 when anything failed so CI can
// tell a partial batch from a clean one.
func exitForBatch(tally batchTally) {
	if tally.failed > 0 {
		exitFunc(1)
	}
}
