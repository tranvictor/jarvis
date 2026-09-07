package util

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util/addrbook"
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

const doStuffABIJSON = `[{"inputs":[
	{"internalType":"address","name":"to","type":"address"},
	{"internalType":"uint256","name":"amount","type":"uint256"},
	{"internalType":"address[]","name":"spenders","type":"address[]"}
],"name":"doStuff","outputs":[],"stateMutability":"nonpayable","type":"function"}]`

func TestPromptFunctionCallDataEchoesCompactForm(t *testing.T) {
	const (
		contract = "0x1234567890123456789012345678901234567890"
		vitalik  = "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045"
		alice    = "0xaaaa000000000000000000000000000000001111"
		bob      = "0xbbbb000000000000000000000000000000002222"
	)
	resolver := addrbook.Map{strings.ToLower(vitalik): "Vitalik Buterin", strings.ToLower(alice): "alice"}
	analyzer := txanalyzer.NewGenericAnalyzerWithContext(
		txanalyzer.NewAnalysisContextWithResolver(nil, networks.EthereumMainnet, resolver),
	)

	rec := ui.NewRecordingUI(
		"1",                    // method index
		"not-an-address",       // invalid answer for `to`
		vitalik,                // valid answer for `to`
		"1500",                 // amount
		"["+alice+", "+bob+"]", // spenders
	)
	method, params, err := PromptFunctionCallData(
		rec, analyzer, contract, 0, nil, false, "write",
		mustABI(t, doStuffABIJSON), nil, networks.EthereumMainnet,
	)
	if err != nil {
		t.Fatalf("PromptFunctionCallData: %v", err)
	}
	if method.Name != "doStuff" || len(params) != 3 {
		t.Fatalf("got method %q with %d params", method.Name, len(params))
	}

	var got []string
	for _, e := range rec.Entries() {
		if e.Method == "Ask" || e.Method == "Choose" {
			continue
		}
		got = append(got, e.Method+": "+e.Value)
	}
	want := []string{
		"Info: write functions:",
		"Info: 1. doStuff",
		"Info: Please choose method index [1, 1]",
		"Info: doStuff  →  0x1234567890123456789012345678901234567890",
		"Info: 1. to  address",
		"Error: ✗ ",
		"Info: 1. to  address", // label repeats so the retry prompt is labelled
		"Rewrite:   → Vitalik Buterin (0xd8dA…6045)",
		"Info: 2. amount  uint256",
		"Rewrite:   → 1500",
		"Info: 3. spenders  address[]",
		// The bracketed answer is longer than foldableInputLen, so it is not
		// folded into the "> answer" row.
		"Info:   → [2 items]",
		"Info:     ├─ alice (0xaAaA…1111)",
		"Info:     └─ 0xBbbb…2222",
	}
	if len(got) != len(want) {
		t.Fatalf("entry count %d != %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Fatalf("entry %d: got %q, want prefix %q\nall:\n%s", i, got[i], want[i], strings.Join(got, "\n"))
		}
	}
	if rec.HasMessage("You entered") || rec.HasMessage("Parameter | Value") {
		t.Fatalf("compact echo must replace the old table: %v", rec.Entries())
	}
}

func TestPromptFunctionCallDataPrefillDoesNotFold(t *testing.T) {
	const contract = "0x1234567890123456789012345678901234567890"
	analyzer := txanalyzer.NewGenericAnalyzerWithContext(
		txanalyzer.NewAnalysisContextWithResolver(nil, networks.EthereumMainnet, addrbook.Map{}),
	)
	rec := ui.NewRecordingUI("42")
	_, params, err := PromptFunctionCallData(
		rec, analyzer, contract, 1,
		[]string{"0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045", "?", "[]"}, true, "write",
		mustABI(t, doStuffABIJSON), nil, networks.EthereumMainnet,
	)
	if err != nil {
		t.Fatalf("PromptFunctionCallData: %v", err)
	}
	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(params))
	}
	var rewrites, infos []string
	for _, e := range rec.Entries() {
		switch e.Method {
		case "Rewrite":
			rewrites = append(rewrites, e.Value)
		case "Info":
			infos = append(infos, e.Value)
		}
	}
	// Only the interactively answered slot ("?") had a "> answer" row to fold.
	if len(rewrites) != 1 || rewrites[0] != "  → 42" {
		t.Fatalf("expected exactly one folded echo for the prompted slot, got %v", rewrites)
	}
	joined := strings.Join(infos, "\n")
	if !strings.Contains(joined, "  → 0xd8dA…6045") || !strings.Contains(joined, "  → [0 items]") {
		t.Fatalf("prefilled slots should be echoed as plain lines:\n%s", joined)
	}
}
