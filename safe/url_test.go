package safe

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestParseSafeAppURL(t *testing.T) {
	const (
		safeHex = "0x71f8f067348d47cced223eA24D2D77235bea722B"
		hashHex = "0x82c28e25b40c865440a0e89fd9578fe62f629f5fafaa0af0587342f7b4b41efe"
	)
	wantSafe := common.HexToAddress(safeHex)
	wantHash := common.HexToHash(hashHex)

	tests := []struct {
		name           string
		input          string
		wantOK         bool
		wantSafe       common.Address
		wantHasHash    bool
		wantHashHex    string
		wantShortName  string
		wantChainID    uint64
	}{
		{
			name:          "full transaction URL",
			input:         "https://app.safe.global/transactions/tx?id=multisig_" + safeHex + "_" + hashHex + "&safe=eth:" + safeHex,
			wantOK:        true,
			wantSafe:      wantSafe,
			wantHasHash:   true,
			wantHashHex:   strings.ToLower(hashHex),
			wantShortName: "eth",
			wantChainID:   1,
		},
		{
			name:          "queue page (no hash)",
			input:         "https://app.safe.global/transactions/queue?safe=arb1:" + safeHex,
			wantOK:        true,
			wantSafe:      wantSafe,
			wantHasHash:   false,
			wantShortName: "arb1",
			wantChainID:   42161,
		},
		{
			name:          "home page",
			input:         "https://app.safe.global/home?safe=base:" + safeHex,
			wantOK:        true,
			wantSafe:      wantSafe,
			wantHasHash:   false,
			wantShortName: "base",
			wantChainID:   8453,
		},
		{
			name:          "EIP-3770 short form",
			input:         "matic:" + safeHex,
			wantOK:        true,
			wantSafe:      wantSafe,
			wantHasHash:   false,
			wantShortName: "matic",
			wantChainID:   137,
		},
		{
			name:     "bare address",
			input:    safeHex,
			wantOK:   true,
			wantSafe: wantSafe,
		},
		{
			name:          "URL with leading/trailing whitespace",
			input:         "   https://app.safe.global/transactions/tx?id=multisig_" + safeHex + "_" + hashHex + "&safe=eth:" + safeHex + "   ",
			wantOK:        true,
			wantSafe:      wantSafe,
			wantHasHash:   true,
			wantHashHex:   strings.ToLower(hashHex),
			wantShortName: "eth",
			wantChainID:   1,
		},
		{
			name:        "bare multisig id token",
			input:       "multisig_" + safeHex + "_" + hashHex,
			wantOK:      true,
			wantSafe:    wantSafe,
			wantHasHash: true,
			wantHashHex: strings.ToLower(hashHex),
		},
		{
			name:        "id= prefixed token (shell-mangled URL)",
			input:       "id=multisig_" + safeHex + "_" + hashHex,
			wantOK:      true,
			wantSafe:    wantSafe,
			wantHasHash: true,
			wantHashHex: strings.ToLower(hashHex),
		},
		{
			name:        "URL truncated by unquoted &",
			input:       "https://app.safe.global/transactions/tx?id=multisig_" + safeHex + "_" + hashHex,
			wantOK:      true,
			wantSafe:    wantSafe,
			wantHasHash: true,
			wantHashHex: strings.ToLower(hashHex),
		},
		{
			name:          "id token with stray chain hint recovers chain",
			input:         "id=multisig_" + safeHex + "_" + hashHex + " safe=eth:" + safeHex,
			wantOK:        true,
			wantSafe:      wantSafe,
			wantHasHash:   true,
			wantHashHex:   strings.ToLower(hashHex),
			wantShortName: "eth",
			wantChainID:   1,
		},
		{
			name:   "non-safe URL is rejected",
			input:  "https://etherscan.io/tx/" + hashHex,
			wantOK: false,
		},
		{
			name:        "wrong host but unambiguous multisig id token still parses",
			input:       "https://etherscan.io/?id=multisig_" + safeHex + "_" + hashHex,
			wantOK:      true,
			wantSafe:    wantSafe,
			wantHasHash: true,
			wantHashHex: strings.ToLower(hashHex),
		},
		{
			name:   "wrong host with only safe= query (no id token) is rejected",
			input:  "https://etherscan.io/?safe=eth:" + safeHex,
			wantOK: false,
		},
		{
			name:   "garbage",
			input:  "hello world",
			wantOK: false,
		},
		{
			name:   "empty",
			input:  "",
			wantOK: false,
		},
		{
			name:          "unknown chain short name still parses (chain id stays 0)",
			input:         "weirdchain:" + safeHex,
			wantOK:        true,
			wantSafe:      wantSafe,
			wantShortName: "weirdchain",
			wantChainID:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseSafeAppURL(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got=%+v)", ok, tc.wantOK, got)
			}
			if !ok {
				return
			}
			if got.SafeAddress != tc.wantSafe {
				t.Errorf("SafeAddress = %s, want %s", got.SafeAddress.Hex(), tc.wantSafe.Hex())
			}
			if got.HasTxHash() != tc.wantHasHash {
				t.Errorf("HasTxHash = %v, want %v", got.HasTxHash(), tc.wantHasHash)
			}
			if tc.wantHasHash {
				if h := got.SafeTxHashHex(); strings.ToLower(h) != tc.wantHashHex {
					t.Errorf("SafeTxHashHex = %s, want %s", h, tc.wantHashHex)
				}
				if got.SafeTxHash != [32]byte(wantHash) {
					t.Errorf("SafeTxHash bytes mismatch")
				}
			}
			if got.ChainShortName != tc.wantShortName {
				t.Errorf("ChainShortName = %q, want %q", got.ChainShortName, tc.wantShortName)
			}
			if got.ChainID != tc.wantChainID {
				t.Errorf("ChainID = %d, want %d", got.ChainID, tc.wantChainID)
			}
		})
	}
}
