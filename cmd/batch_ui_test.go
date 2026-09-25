package cmd

import (
	"strings"
	"testing"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/ui"
)

func swapAppUI(t *testing.T, rec *ui.RecordingUI) {
	t.Helper()
	prev := appUI
	appUI = rec
	t.Cleanup(func() { appUI = prev })
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

	prevCont := batchContinueOnError
	t.Cleanup(func() { batchContinueOnError = prevCont })
	batchContinueOnError = true
	rec = ui.NewRecordingUI()
	swapAppUI(t, rec)
	if !continueBatchAfterFailure(2) {
		t.Fatal("--continue-on-error must never stop the batch")
	}
	if len(rec.Entries()) != 0 {
		t.Fatalf("--continue-on-error must not prompt: %v", rec.Entries())
	}
}

// fakeSafeRefs swaps reviewSafeRefFn/signSafeRefFn for stubs: refs whose
// token contains "bad" fail review, everything else is signed successfully
// unless its token contains "reject", which fails at signing time.
func fakeSafeRefs(t *testing.T) *[]string {
	t.Helper()
	prevReview, prevSign := reviewSafeRefFn, signSafeRefFn
	t.Cleanup(func() { reviewSafeRefFn, signSafeRefFn = prevReview, prevSign })

	signed := &[]string{}
	reviewSafeRefFn = func(in safeRefInput) (*preparedSafeRef, approveSafeRefResult) {
		res := approveSafeRefResult{network: "mainnet", safeTxHash: "0x" + in.original}
		if strings.Contains(in.original, "bad") {
			res.status, res.reason = "failed", "fetch pending tx: 404"
			appUI.Error("%s", res.reason)
			return nil, res
		}
		appUI.Info("card for %s", in.original)
		return &preparedSafeRef{in: in, res: res}, res
	}
	signSafeRefFn = func(p *preparedSafeRef, preConfirmed bool) approveSafeRefResult {
		if !preConfirmed {
			t.Fatalf("confirm-once must pre-confirm %s", p.in.original)
		}
		*signed = append(*signed, p.in.original)
		res := p.res
		if strings.Contains(p.in.original, "reject") {
			res.status, res.reason = "failed", "sign safeTxHash: rejected on device"
			return res
		}
		res.status, res.confirmType = "approved", "approve"
		return res
	}
	return signed
}

func safeRefInputs(tokens ...string) []safeRefInput {
	out := make([]safeRefInput, len(tokens))
	for i, tok := range tokens {
		out[i] = safeRefInput{original: tok}
	}
	return out
}

func TestApproveSafeRefsConfirmOnceReviewsThenSigns(t *testing.T) {
	prevYes := config.YesToAllPrompt
	t.Cleanup(func() { config.YesToAllPrompt = prevYes })
	config.YesToAllPrompt = false

	signed := fakeSafeRefs(t)
	rec := ui.NewRecordingUI("y", "")
	swapAppUI(t, rec)
	t.Cleanup(cmdutil.ClearBatchItem)

	tally := batchTally{total: 4}
	results, item, aborted := approveSafeRefsConfirmOnce(safeRefInputs("a", "bad", "reject", "d"), &tally)
	if item != 4 || aborted {
		t.Fatalf("item=%d aborted=%v", item, aborted)
	}
	if got := strings.Join(*signed, ","); got != "a,reject,d" {
		t.Fatalf("signed %q", got)
	}
	wantStatus := []string{"approved", "failed", "failed", "approved"}
	for i, r := range results {
		if r.status != wantStatus[i] {
			t.Fatalf("result %d = %s, want %s (%+v)", i, r.status, wantStatus[i], r)
		}
	}
	if tally.ok != 2 || tally.failed != 2 || tally.done() != 4 {
		t.Fatalf("tally %+v", tally)
	}

	var transcript []string
	for _, e := range rec.Entries() {
		transcript = append(transcript, e.Method+": "+e.Value)
	}
	joined := strings.Join(transcript, "\n")
	// Every card is shown before the single confirm, and the failed review
	// is reported while still in the review pass.
	confirmAt := strings.Index(joined, "Confirm: Sign all 3 reviewed Safe approval(s)")
	if confirmAt < 0 {
		t.Fatalf("single confirm missing:\n%s", joined)
	}
	for _, s := range []string{"card for a", "card for reject", "card for d", "fetch pending tx: 404", "✗ [2/4] failed  fetch pending tx: 404"} {
		at := strings.Index(joined, s)
		if at < 0 || at > confirmAt {
			t.Fatalf("%q should appear before the confirm:\n%s", s, joined)
		}
	}
	if !rec.HasMessage("Continue with the remaining 1 transaction(s)?") {
		t.Fatalf("failed signing should ask to continue:\n%s", joined)
	}
	if strings.Count(joined, "Confirm: Sign approval") != 0 {
		t.Fatalf("per-item confirms must not appear:\n%s", joined)
	}
}

func TestApproveSafeRefsConfirmOnceDeclineSkipsAll(t *testing.T) {
	prevYes := config.YesToAllPrompt
	t.Cleanup(func() { config.YesToAllPrompt = prevYes })
	config.YesToAllPrompt = false

	signed := fakeSafeRefs(t)
	rec := ui.NewRecordingUI("n")
	swapAppUI(t, rec)
	t.Cleanup(cmdutil.ClearBatchItem)

	tally := batchTally{total: 2}
	results, _, aborted := approveSafeRefsConfirmOnce(safeRefInputs("a", "b"), &tally)
	if aborted {
		t.Fatal("declining is not an abort of later Classic items")
	}
	if len(*signed) != 0 {
		t.Fatalf("nothing should be signed after n: %v", *signed)
	}
	for _, r := range results {
		if r.status != "skipped" || r.reason != "user aborted" {
			t.Fatalf("unexpected result %+v", r)
		}
	}
	if tally.skipped != 2 {
		t.Fatalf("tally %+v", tally)
	}
}

func TestApproveSafeRefsConfirmOnceYesSkipsPrompt(t *testing.T) {
	prevYes := config.YesToAllPrompt
	t.Cleanup(func() { config.YesToAllPrompt = prevYes })
	config.YesToAllPrompt = true

	signed := fakeSafeRefs(t)
	rec := ui.NewRecordingUI()
	swapAppUI(t, rec)
	t.Cleanup(cmdutil.ClearBatchItem)

	tally := batchTally{total: 1}
	approveSafeRefsConfirmOnce(safeRefInputs("a"), &tally)
	if len(*signed) != 1 {
		t.Fatalf("signed %v", *signed)
	}
	for _, e := range rec.Entries() {
		if e.Method == "Confirm" {
			t.Fatalf("--yes must not prompt: %v", e)
		}
	}
}

func TestExitForBatchOnlyFailsOnFailedItems(t *testing.T) {
	code := -1
	prevExit := exitFunc
	exitFunc = func(c int) { code = c }
	t.Cleanup(func() { exitFunc = prevExit })
	tally := batchTally{total: 3}
	for _, s := range []string{"approved", "failed", "skipped"} {
		tally.add(s)
	}
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
