package util

import (
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/vet"
)

const (
	vetDelegate = cardRouter
	vetUnknown  = cardMe
)

func vetBook() map[string]string {
	return map[string]string{strings.ToLower(vetDelegate): "Batch Delegator"}
}

func nameOne(text string) string {
	return nameFindings([]vet.Finding{{Text: text}}, vetBook())[0].Text
}

func TestNameFindingsAddsAddressBookName(t *testing.T) {
	got := nameOne("EIP-7702: destination delegates execution to " + vetDelegate + "; that contract's code runs for this account")
	want := "EIP-7702: destination delegates execution to " + vetDelegate + " (Batch Delegator); that contract's code runs for this account"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestNameFindingsMatchesAnyHexCasing(t *testing.T) {
	got := nameOne("delegates to " + strings.ToLower(vetDelegate))
	if got != "delegates to "+strings.ToLower(vetDelegate)+" (Batch Delegator)" {
		t.Fatalf("lowercase hex not named: %q", got)
	}
}

func TestNameFindingsLeavesUnknownAddressesBare(t *testing.T) {
	text := "token recipient " + vetUnknown + " is not in your address book"
	if got := nameOne(text); got != text {
		t.Fatalf("unknown address changed: %q", got)
	}
}

func TestNameFindingsDoesNotNameTwice(t *testing.T) {
	text := "lookalike of " + vetDelegate + " (Batch Delegator)"
	if got := nameOne(text); got != text {
		t.Fatalf("already-named address annotated again: %q", got)
	}
}

func TestNameFindingsIgnoresHexInsideLongerBlobs(t *testing.T) {
	blob := "0x" + strings.Repeat("ab", 12) + strings.ToLower(vetDelegate[2:])
	text := "calldata " + blob + " was not decoded"
	if got := nameOne(text); got != text {
		t.Fatalf("hex inside a longer blob was named: %q", got)
	}
}

func TestNameFindingsHonoursMaskNames(t *testing.T) {
	config.MaskNames = true
	t.Cleanup(func() { config.MaskNames = false })
	got := nameOne("delegates to " + vetDelegate)
	if strings.Contains(got, "Batch Delegator") || !strings.Contains(got, vetDelegate+" (•••)") {
		t.Fatalf("--mask-names should hide the label: %q", got)
	}
}

func TestNameFindingsNamesEveryAddressInAFinding(t *testing.T) {
	text := "EIP-7702 authorization: " + vetUnknown + " grants " + vetDelegate + " full control of the account until revoked"
	got := nameOne(text)
	want := "EIP-7702 authorization: " + vetUnknown + " grants " + vetDelegate + " (Batch Delegator) full control of the account until revoked"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}
