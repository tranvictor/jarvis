package util

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/tranvictor/jarvis/ui"
)

// SafeProxy verified ABI: constructor + fallback only. This is what
// `jarvis contract read` used to turn into "Please choose method index [1, 0]".
const safeProxyABIJSON = `[{"inputs":[{"internalType":"address","name":"_singleton","type":"address"}],"stateMutability":"nonpayable","type":"constructor"},{"stateMutability":"payable","type":"fallback"}]`

const mixedABIJSON = `[
	{"inputs":[],"name":"getOwners","outputs":[{"internalType":"address[]","name":"","type":"address[]"}],"stateMutability":"view","type":"function"},
	{"inputs":[],"name":"VERSION","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"pure","type":"function"},
	{"inputs":[{"internalType":"address","name":"owner","type":"address"}],"name":"addOwner","outputs":[],"stateMutability":"nonpayable","type":"function"}
]`

func mustABI(t *testing.T, jsonStr string) *abi.ABI {
	t.Helper()
	parsed, err := abi.JSON(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("parse ABI: %v", err)
	}
	return &parsed
}

func TestPromptMethodEmptyReadMethods(t *testing.T) {
	rec := ui.NewRecordingUI()
	_, _, err := PromptMethod(rec, mustABI(t, safeProxyABIJSON), 0, "read")
	if err == nil {
		t.Fatal("expected error for a methodless proxy ABI, got nil")
	}
	if !strings.Contains(err.Error(), "no read methods") {
		t.Fatalf("error %q should mention that there are no read methods", err)
	}
	if rec.HasMessage("Please choose method index") {
		t.Fatal("must not prompt for a method index when the list is empty")
	}
}

func TestPromptMethodEmptyWriteMethods(t *testing.T) {
	const viewOnly = `[{"inputs":[],"name":"name","outputs":[{"type":"string","name":""}],"stateMutability":"view","type":"function"}]`
	rec := ui.NewRecordingUI()
	_, _, err := PromptMethod(rec, mustABI(t, viewOnly), 0, "write")
	if err == nil {
		t.Fatal("expected error when the ABI has no write methods")
	}
	if !strings.Contains(err.Error(), "no write methods") {
		t.Fatalf("error %q should mention that there are no write methods", err)
	}
}

func TestPromptMethodSelectsReadByIndex(t *testing.T) {
	rec := ui.NewRecordingUI("1")
	method, name, err := PromptMethod(rec, mustABI(t, mixedABIJSON), 0, "read")
	if err != nil {
		t.Fatalf("PromptMethod: %v", err)
	}
	// Read methods are sorted by name: VERSION, getOwners
	if name != "VERSION" || method == nil || method.Name != "VERSION" {
		t.Fatalf("got method %q, want VERSION", name)
	}
	if !rec.HasMessage("Please choose method index [1, 2]") {
		t.Fatalf("prompt should show a valid [1, 2] range, entries=%v", rec.InfoMessages())
	}
}

func TestPromptMethodPrefillIndex(t *testing.T) {
	rec := ui.NewRecordingUI()
	method, name, err := PromptMethod(rec, mustABI(t, mixedABIJSON), 2, "read")
	if err != nil {
		t.Fatalf("PromptMethod: %v", err)
	}
	if name != "getOwners" || method.Name != "getOwners" {
		t.Fatalf("got method %q, want getOwners", name)
	}
}
