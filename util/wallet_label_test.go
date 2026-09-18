package util

import (
	"strings"
	"testing"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util/addrbook"
)

func TestApplyWalletLabel(t *testing.T) {
	addr := "0xa3759774994F5012E5d725dCC1B96750945C793f"
	cases := []struct {
		name, desc, walletDesc, walletKind, want string
	}{
		{"address book name", "Alice", "work ledger", "ledger", "Alice - your wallet"},
		{"wallet description only", "unknown", "work ledger", "ledger", "work ledger - your wallet"},
		{"wallet kind fallback", "unknown", "", "ledger", "ledger - your wallet"},
		{"unnamed wallet", "unknown", "", "", "your wallet"},
		{"already tagged", "Alice - your wallet", "x", "ledger", "Alice - your wallet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := applyWalletLabel(jarviscommon.Address{Address: addr, Desc: tc.desc}, tc.walletDesc, tc.walletKind)
			if got.Desc != tc.want {
				t.Fatalf("Desc = %q, want %q", got.Desc, tc.want)
			}
			if !jarviscommon.IsKnownAddress(got) {
				t.Fatal("wallet label must count as a known address")
			}
			if !got.Private {
				t.Fatal("wallet label must be marked private so --mask-names can hide it")
			}
			plain := jarviscommon.PlainAddress(got)
			if !strings.Contains(plain, addr) || !strings.Contains(plain, "("+tc.want+")") {
				t.Fatalf("PlainAddress = %q", plain)
			}
		})
	}
}

func TestApplyWalletLabelSkipsZeroAddress(t *testing.T) {
	zero := jarviscommon.Address{Address: "0x0000000000000000000000000000000000000000", Desc: "zero address"}
	got := applyWalletLabel(zero, "me", "ledger")
	if got.Desc != "zero address" {
		t.Fatalf("zero address must not become a wallet: %+v", got)
	}
}

func TestEnrichedResolverAnnotatesWallets(t *testing.T) {
	const (
		booked = "0x69694c738fe96c41ecce3588fa5759d717cf9cc6"
		mine   = "0xa3759774994F5012E5d725dCC1B96750945C793f"
	)
	prev := walletNamer
	SetWalletNamer(func(addr string) (string, string, bool) {
		switch strings.ToLower(addr) {
		case strings.ToLower(booked), strings.ToLower(mine):
			return "work ledger", "ledger", true
		}
		return "", "", false
	})
	t.Cleanup(func() { SetWalletNamer(prev) })

	r := &EnrichedResolver{
		inner: addrbook.Map{
			strings.ToLower(booked): "Alice",
		},
	}
	got := r.Resolve(booked)
	if got.Desc != "Alice - your wallet" {
		t.Fatalf("booked wallet: %q", got.Desc)
	}
	got = r.Resolve(mine)
	if got.Desc != "work ledger - your wallet" {
		t.Fatalf("unbooked wallet: %q", got.Desc)
	}
	plain := jarviscommon.PlainAddress(got)
	if !strings.Contains(plain, mine) || !strings.Contains(plain, "your wallet") {
		t.Fatalf("plain: %q", plain)
	}
}

func TestApplyWalletLabelMaskNames(t *testing.T) {
	prev := config.MaskNames
	config.MaskNames = true
	t.Cleanup(func() { config.MaskNames = prev })

	addr := "0xa3759774994F5012E5d725dCC1B96750945C793f"
	got := applyWalletLabel(jarviscommon.Address{Address: addr, Desc: "Alice"}, "work ledger", "ledger")
	if got.Desc != "Alice - your wallet" {
		t.Fatalf("stored Desc must stay intact, got %q", got.Desc)
	}
	if !got.Private {
		t.Fatal("expected Private")
	}
	plain := jarviscommon.PlainAddress(got)
	if strings.Contains(plain, "Alice") || strings.Contains(plain, "work ledger") {
		t.Fatalf("masked PlainAddress leaked a name: %q", plain)
	}
	if !strings.Contains(plain, addr) || !strings.Contains(plain, "("+jarviscommon.MaskedName+")") {
		t.Fatalf("masked PlainAddress = %q", plain)
	}
}
