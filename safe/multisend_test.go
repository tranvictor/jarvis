package safe

import (
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/networks"
)

// TestMultiSendCandidateOrder pins the probe order: the release matching the
// Safe's own version is tried first, but the other release stays in the list
// because MultiSendCallOnly is a standalone stateless library that works with
// any Safe version, and a chain may only have one of them deployed.
func TestMultiSendCandidateOrder(t *testing.T) {
	const (
		v141 = "0x9641d764fc13c8B624c04430C7356C1C7C8102e2"
		v130 = "0x40A2aCCbd92BCA938b02010E17A5b8929b49130D"
	)

	cases := map[string]string{
		"1.4.1":   v141,
		"1.4.0":   v141,
		" 1.4.1 ": v141,
		"1.3.0":   v130,
		"1.1.1":   v130,
		"":        v130, // VERSION() unreadable
		"weird":   v130,
	}

	for version, wantFirst := range cases {
		got := multiSendCandidatesFor(version)
		if len(got) < 3 {
			t.Errorf("version %q: only %d candidates, expected every known deployment", version, len(got))
		}
		if !strings.EqualFold(got[0].Address, wantFirst) {
			t.Errorf("version %q: first candidate = %s, want %s", version, got[0].Address, wantFirst)
		}
		seen := map[string]bool{}
		for _, c := range got {
			key := strings.ToLower(c.Address)
			if seen[key] {
				t.Errorf("version %q: duplicate candidate %s", version, c.Address)
			}
			seen[key] = true
			if c.Label == "" {
				t.Errorf("version %q: candidate %s has no label", version, c.Address)
			}
		}
	}
}

// TestMultiSendCandidatesAreNotAliased guards against the append-to-shared-slice
// bug: multiSendCandidatesFor must never mutate the package-level tables.
func TestMultiSendCandidatesAreNotAliased(t *testing.T) {
	before141 := len(multiSendCallOnly141)
	before130 := len(multiSendCallOnly130)

	_ = multiSendCandidatesFor("1.4.1")
	_ = multiSendCandidatesFor("1.3.0")

	if len(multiSendCallOnly141) != before141 || len(multiSendCallOnly130) != before130 {
		t.Fatalf("candidate tables were mutated: %d/%d -> %d/%d",
			before141, before130, len(multiSendCallOnly141), len(multiSendCallOnly130))
	}
}

func TestResolveMultiSendCallOnlyOverride(t *testing.T) {
	addr, label, err := ResolveMultiSendCallOnly(nil, networks.BSCMainnet, "  0x000000000000000000000000000000000000dEaD ")
	if err != nil {
		t.Fatalf("override: %s", err)
	}
	if addr.Hex() != "0x000000000000000000000000000000000000dEaD" {
		t.Errorf("address = %s", addr.Hex())
	}
	if label == "" {
		t.Error("expected a label describing the override")
	}

	if _, _, err := ResolveMultiSendCallOnly(nil, networks.BSCMainnet, "not-an-address"); err == nil {
		t.Error("expected an error for a malformed --multisend-address")
	}
}
