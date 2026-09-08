package util

import "fmt"

// batchItemMark is "[i/n]" while `jarvis msig bapprove` is processing one
// item. Empty outside a batch so a single approve/sign is unchanged.
var batchItemMark string

// SetBatchItem stamps every signing card, prompt and result line with
// [i/n] until the next SetBatchItem or ClearBatchItem. That way a
// 50-item transcript stays scannable: every equals-rule and Y/n prompt
// names the item you are looking at, not just the banner that scrolled
// off the top.
func SetBatchItem(i, total int) {
	if i < 1 || total < 1 {
		batchItemMark = ""
		return
	}
	batchItemMark = fmt.Sprintf("[%d/%d]", i, total)
}

// ClearBatchItem drops the current [i/n] stamp. Call when the batch ends
// so a later signing card in the same process is not mislabelled.
func ClearBatchItem() { batchItemMark = "" }

// AnnotateBatch prefixes s with the current [i/n] mark. No-op outside a
// batch or when s is empty.
func AnnotateBatch(s string) string {
	if batchItemMark == "" || s == "" {
		return s
	}
	return batchItemMark + " " + s
}
