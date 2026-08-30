package safe

import (
	"errors"
	"testing"
)

func TestNextFreeNonceEmptyQueue(t *testing.T) {
	got, err := nextFreeNonce(5, func(uint64) (*PendingTx, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 5 {
		t.Fatalf("got %d, want 5", got)
	}
}

func TestNextFreeNonceSkipsOccupied(t *testing.T) {
	occupied := map[uint64]bool{5: true, 6: true}
	got, err := nextFreeNonce(5, func(nonce uint64) (*PendingTx, error) {
		if occupied[nonce] {
			return &PendingTx{}, nil
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
}

func TestNextFreeNonceFirstLookupError(t *testing.T) {
	_, err := nextFreeNonce(3, func(uint64) (*PendingTx, error) {
		return nil, errors.New("service down")
	})
	if err == nil {
		t.Fatal("expected error on first lookup failure")
	}
}

func TestNextFreeNonceLaterLookupErrorStopsWalk(t *testing.T) {
	got, err := nextFreeNonce(1, func(nonce uint64) (*PendingTx, error) {
		if nonce == 1 {
			return &PendingTx{}, nil
		}
		return nil, errors.New("gone")
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("got %d, want 2 (walk stops after a later lookup error)", got)
	}
}

func TestNextFreeNonceCapsWalk(t *testing.T) {
	got, err := nextFreeNonce(10, func(uint64) (*PendingTx, error) {
		return &PendingTx{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 74 {
		t.Fatalf("got %d, want 74 (start 10 + 64 occupied slots)", got)
	}
}
