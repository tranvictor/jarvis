package cmd

import (
	"math/big"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
)

func TestParseNewMsigType(t *testing.T) {
	cases := []struct {
		in      string
		want    cmdutil.MultisigType
		wantErr bool
	}{
		{"safe", cmdutil.MultisigSafe, false},
		{" SAFE ", cmdutil.MultisigSafe, false},
		{"gnosis-safe", cmdutil.MultisigSafe, false},
		{"classic", cmdutil.MultisigClassic, false},
		{"gnosis-classic", cmdutil.MultisigClassic, false},
		{"", cmdutil.MultisigUnknown, false},
		{"ledger", cmdutil.MultisigUnknown, true},
	}
	for _, tc := range cases {
		got, err := parseNewMsigType(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %s", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseSafeDeployParams(t *testing.T) {
	a := ethcommon.HexToAddress("0x1111111111111111111111111111111111111111")
	b := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	owners, threshold, salt, err := parseSafeDeployParams([]any{
		[]ethcommon.Address{a, b},
		big.NewInt(2),
		uint64(9),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(owners) != 2 || owners[0] != a || owners[1] != b {
		t.Errorf("owners = %v", owners)
	}
	if threshold.Cmp(big.NewInt(2)) != 0 {
		t.Errorf("threshold = %s", threshold)
	}
	if salt.Cmp(big.NewInt(9)) != 0 {
		t.Errorf("salt = %s", salt)
	}

	if _, _, _, err := parseSafeDeployParams([]any{a}); err == nil {
		t.Error("expected error for wrong arity")
	}
	if _, _, _, err := parseSafeDeployParams([]any{"nope", big.NewInt(1), big.NewInt(0)}); err == nil {
		t.Error("expected error for bad owners")
	}
}
