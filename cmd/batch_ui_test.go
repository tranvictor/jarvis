package cmd

import (
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/ui"
)

func swapAppUI(t *testing.T, rec *ui.RecordingUI) {
	t.Helper()
	prev := appUI
	appUI = rec
	t.Cleanup(func() { appUI = prev })
}

func TestBatchTallyString(t *testing.T) {
	tl := batchTally{total: 4}
	if tl.String() != "0 ok · 4 left" {
		t.Fatalf("empty: %q", tl.String())
	}
	tl.add("approved")
	tl.add("executed")
	tl.add("skipped")
	if tl.String() != "2 ok · 1 skipped · 1 left" {
		t.Fatalf("partial: %q", tl.String())
	}
	tl.add("failed")
	if tl.String() != "2 ok · 1 skipped · 1 failed" {
		t.Fatalf("complete: %q", tl.String())
	}
	if tl.done() != 4 {
		t.Fatalf("done = %d", tl.done())
	}
}

func TestBatchPlanBannerAndResultLines(t *testing.T) {
	rec := ui.NewRecordingUI()
	swapAppUI(t, rec)

	printBatchPlan("Batch approve", []string{"Safe     eth:0xsafe:0xhash", "Classic  mainnet:0xinit"})
	tally := batchTally{total: 2}
	printBatchBanner(1, 2, "Safe", "eth:0xsafe:0xhash")
	withIndentedUI(func() { appUI.Info("inner") })
	tally.add("approved")
	printBatchItemResult("approved", "safeTxHash 0xhash", tally)
	tally.add("failed")
	printBatchItemResult("failed", "sign safeTxHash: rejected", tally)

	var got []string
	for _, e := range rec.Entries() {
		got = append(got, e.Method+": "+e.Value)
	}
	want := []string{
		"Section: Batch approve: 2 transaction(s)",
		"Info: 1. Safe     eth:0xsafe:0xhash",
		"Info: 2. Classic  mainnet:0xinit",
		"Info: ",
		"Info: [1/2] Safe  eth:0xsafe:0xhash",
		"Info: inner",
		"Info: ✓ approved  safeTxHash 0xhash   (1 ok · 1 left)",
		"Info: ✗ failed  sign safeTxHash: rejected   (1 ok · 1 failed)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestContinueBatchAfterFailure(t *testing.T) {
	prevYes := config.YesToAllPrompt
	t.Cleanup(func() { config.YesToAllPrompt = prevYes })

	config.YesToAllPrompt = true
	swapAppUI(t, ui.NewRecordingUI())
	if !continueBatchAfterFailure(3) {
		t.Fatal("--yes must never stop the batch")
	}

	config.YesToAllPrompt = false
	if !continueBatchAfterFailure(0) {
		t.Fatal("nothing left to do must not prompt")
	}
	rec := ui.NewRecordingUI("n")
	swapAppUI(t, rec)
	if continueBatchAfterFailure(2) {
		t.Fatal("answering n must stop the batch")
	}
	if !rec.HasMessage("Continue with the remaining 2 transaction(s)?") {
		t.Fatalf("prompt missing: %v", rec.Entries())
	}
}

func TestPrintBatchApproveSummaryAndExitCode(t *testing.T) {
	rec := ui.NewRecordingUI()
	swapAppUI(t, rec)
	prevDegen := config.DegenMode
	config.DegenMode = false
	t.Cleanup(func() { config.DegenMode = prevDegen })

	safeResults := []safeBatchResult{
		{network: "mainnet", safeAddress: "0xsafe", safeTxHash: "0xhash", status: "approved"},
		{network: "bsc", safeAddress: "0xsafe2", status: "failed", reason: "unlock wallet: nope"},
	}
	classic := []batchResult{
		{network: "matic", initTxHash: "0xinit", msigTxID: "7", status: "skipped", reason: "already executed"},
	}
	tally := batchTally{total: 3}
	for _, r := range safeResults {
		tally.add(r.status)
	}
	tally.add(classic[0].status)
	printBatchApproveSummary(safeResults, classic, tally)

	for _, w := range []string{
		"Section: Batch summary",
		"Table: # | Kind | Network | Target | Result | Detail",
		"Table: 1 | Safe | mainnet | 0xsafe | approved | safeTxHash 0xhash",
		"Table: 2 | Safe | bsc | 0xsafe2 | failed | unlock wallet: nope",
		"Table: 3 | Classic | matic | msig #7 | skipped | already executed",
		"Info: 3 transaction(s): 1 ok · 1 skipped · 1 failed",
	} {
		found := false
		for _, e := range rec.Entries() {
			if e.Method+": "+e.Value == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %q in %v", w, rec.Entries())
		}
	}

	code := -1
	prevExit := exitFunc
	exitFunc = func(c int) { code = c }
	t.Cleanup(func() { exitFunc = prevExit })
	exitForBatch(tally)
	if code != 1 {
		t.Fatalf("a failed item must exit 1, got %d", code)
	}
	code = -1
	exitForBatch(batchTally{total: 2, ok: 1, skipped: 1})
	if code != -1 {
		t.Fatalf("skips alone must not change the exit code, got %d", code)
	}
}
