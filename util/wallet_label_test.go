package util

import (
	"strings"
	"testing"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/util/addrbook"
)

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
