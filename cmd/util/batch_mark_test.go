package util

import "testing"

func TestAnnotateBatch(t *testing.T) {
	t.Cleanup(ClearBatchItem)

	if got := AnnotateBatch("EOA transaction"); got != "EOA transaction" {
		t.Fatalf("outside a batch: %q", got)
	}
	if batchItemMark != "" {
		t.Fatalf("mark should start empty, got %q", batchItemMark)
	}

	SetBatchItem(12, 87)
	if batchItemMark != "[12/87]" {
		t.Fatalf("mark = %q", batchItemMark)
	}
	if got := AnnotateBatch("EOA transaction"); got != "[12/87] EOA transaction" {
		t.Fatalf("kind: %q", got)
	}
	if got := AnnotateBatch("Sign and broadcast (≈ 0.0017 ETH)?"); got != "[12/87] Sign and broadcast (≈ 0.0017 ETH)?" {
		t.Fatalf("prompt: %q", got)
	}
	if got := AnnotateBatch(""); got != "" {
		t.Fatalf("empty stays empty: %q", got)
	}

	SetBatchItem(0, 10)
	if batchItemMark != "" {
		t.Fatalf("invalid index must clear, got %q", batchItemMark)
	}
	SetBatchItem(1, 1)
	if batchItemMark != "[1/1]" {
		t.Fatalf("single item: %q", batchItemMark)
	}
	ClearBatchItem()
	if AnnotateBatch("approved") != "approved" {
		t.Fatalf("cleared mark still annotates")
	}
}
