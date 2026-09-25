package common

import (
	"fmt"
	"testing"
	"time"

	"github.com/tranvictor/jarvis/config"
)

func TestNameFirst(t *testing.T) {
	known := Address{Address: "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D", Desc: "Uniswap V2 Router"}
	unknown := Address{Address: "0x9642b23Ed1E01Df1092B92641051881a322F5D4E", Desc: "unknown"}
	blank := Address{Address: "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"}

	cases := []struct {
		addr Address
		full bool
		want string
	}{
		{known, false, "Uniswap V2 Router (0x7a25…488D)"},
		{known, true, "Uniswap V2 Router (0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D)"},
		{unknown, false, "0x9642…5D4E"},
		{blank, true, "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"},
		{Address{Address: "0x0000000000000000000000000000000000000000"}, false, "0x0000…0000 (zero address)"},
		{Address{Address: "0x0000000000000000000000000000000000000000", Desc: "Quang Le"}, false, "0x0000…0000 (zero address)"},
		{Address{}, false, ""},
	}
	for _, c := range cases {
		if got := NameFirst(c.addr, c.full); got != c.want {
			t.Errorf("NameFirst(%v, %v) = %q, want %q", c.addr, c.full, got, c.want)
		}
	}
}

func TestMaskNamesRewritesResolvedNames(t *testing.T) {
	prev := config.MaskNames
	config.MaskNames = true
	t.Cleanup(func() { config.MaskNames = prev })

	hex := "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"
	named := Address{Address: hex, Desc: "me"}
	unknown := Address{Address: hex, Desc: "unknown"}
	token := Address{Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Desc: "USDC token", Decimal: 6}

	if got := NameFirst(named, false); got != "••• (0x9642…5D4E)" {
		t.Errorf("NameFirst = %q", got)
	}
	if got := PlainAddress(named); got != hex+" (•••)" {
		t.Errorf("PlainAddress = %q", got)
	}
	if got := NameFirst(unknown, false); got != "0x9642…5D4E" {
		t.Errorf("unknown must stay unnamed, got %q", got)
	}
	if got := PlainAddress(token); got != "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48 (••• - 6)" {
		t.Errorf("PlainAddress token = %q", got)
	}
}

func TestPlainAddressOmitsUnknown(t *testing.T) {
	hex := "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"
	if got := PlainAddress(Address{Address: hex, Desc: "unknown"}); got != hex {
		t.Fatalf("unknown desc leaked: %q", got)
	}
	if got := PlainAddress(Address{Address: hex, Desc: "me"}); got != hex+" (me)" {
		t.Fatalf("known desc dropped: %q", got)
	}
	zero := "0x0000000000000000000000000000000000000000"
	if got := PlainAddress(Address{Address: zero, Desc: "Quang Le"}); got != zero+" (zero address)" {
		t.Fatalf("zero address must ignore address-book names, got %q", got)
	}
}

func TestGroupDigits(t *testing.T) {
	cases := map[string]string{
		"1000000000":  "1,000,000,000",
		"1234567.891": "1,234,567.891",
		"-1234567":    "-1,234,567",
		"999":         "999",
		"0.5":         "0.5",
		"abc":         "abc",
	}
	for in, want := range cases {
		if got := GroupDigits(in); got != want {
			t.Errorf("GroupDigits(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCompactAmount(t *testing.T) {
	cases := map[string]string{
		"1000":                          "1,000",
		"1000.5":                        "1,000.5",
		"2552732552.244911963753134392": "2,552,732,552.2449",
		"14.633701431326966591":         "14.6337",
		"0.3121":                        "0.3121",
		"0.051932126031271887":          "0.05193",
		"0.000000123456":                "0.0000001234",
		"1.00001":                       "1",
		"0.0000000000000000001":         "0.0000000000000000001",
		"0":                             "0",
	}
	for in, want := range cases {
		if got := CompactAmount(in); got != want {
			t.Errorf("CompactAmount(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTimestampLabel(t *testing.T) {
	fixed := time.Date(2026, 9, 7, 3, 0, 0, 0, time.UTC)
	prev := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = prev })
	at := func(d time.Duration) string { return fmt.Sprintf("%d", fixed.Add(d).Unix()) }

	cases := []struct {
		name, raw, want string
		ok              bool
	}{
		{"deadline", at(-7 * time.Hour), "2026-09-06 20:00:00 UTC, 7 h ago", true},
		{"deadline", at(30 * time.Minute), "2026-09-07 03:30:00 UTC, in 30 min", true},
		{"validUntil", at(30 * 24 * time.Hour), "2026-10-07 03:00:00 UTC, in 30 days", true},
		{"expiration_time", "1725600000", "2024-09-06 05:20:00 UTC, 2 years ago", true},
		{"unlockTime", at(45 * time.Second), "2026-09-07 03:00:45 UTC, in 45s", true},
		{"deadline", "0", "", false}, // unset
		{"deadline", "115792089237316195423570985008687907853269984665640564039457584007913129639935", "", false},
		{"amount", at(time.Hour), "", false}, // not a time-named parameter
		{"time", at(time.Hour), "", false},   // too generic to trust
		{"deadline", "12345", "", false},     // not plausible as seconds
	}
	for _, c := range cases {
		got, ok := TimestampLabel(c.name, c.raw)
		if ok != c.ok || got != c.want {
			t.Errorf("TimestampLabel(%q, %q) = %q, %v; want %q, %v", c.name, c.raw, got, ok, c.want, c.ok)
		}
	}
}
