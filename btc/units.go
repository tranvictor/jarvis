package btc

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

const SatPerBTC int64 = 100_000_000

// MaxMoney is the consensus cap on any single amount (21M BTC).
const MaxMoney int64 = 21_000_000 * SatPerBTC

var ErrAmount = errors.New("invalid bitcoin amount")

// ParseAmount reads "0.001", "0.001 btc", "100000 sat" or "100000 sats"
// into satoshis. It never goes through floating point: 0.1 + 0.2 style
// rounding must not change what a user is about to sign.
func ParseAmount(s string) (int64, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, fmt.Errorf("%w: empty", ErrAmount)
	}
	unit := "btc"
	for _, u := range []string{"sats", "sat", "satoshis", "satoshi", "btc"} {
		if strings.HasSuffix(s, u) {
			unit = u
			s = strings.TrimSpace(strings.TrimSuffix(s, u))
			break
		}
	}
	if s == "" || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, fmt.Errorf("%w: %q", ErrAmount, s)
	}
	whole, frac, hasFrac := strings.Cut(s, ".")
	if whole == "" && !hasFrac {
		return 0, fmt.Errorf("%w: %q", ErrAmount, s)
	}
	if !allDigits(whole) || (hasFrac && !allDigits(frac)) || (whole == "" && frac == "") {
		return 0, fmt.Errorf("%w: %q", ErrAmount, s)
	}
	if unit != "btc" {
		if hasFrac {
			return 0, fmt.Errorf("%w: satoshis cannot have a fractional part", ErrAmount)
		}
		return parseInt(whole)
	}
	if len(frac) > 8 {
		return 0, fmt.Errorf("%w: %q has more than 8 decimal places", ErrAmount, s)
	}
	frac += strings.Repeat("0", 8-len(frac))
	w, err := parseInt(orZero(whole))
	if err != nil {
		return 0, err
	}
	if w > MaxMoney/SatPerBTC {
		return 0, fmt.Errorf("%w: above 21,000,000 BTC", ErrAmount)
	}
	f, err := parseInt(frac)
	if err != nil {
		return 0, err
	}
	total := w*SatPerBTC + f
	if total > MaxMoney {
		return 0, fmt.Errorf("%w: above 21,000,000 BTC", ErrAmount)
	}
	return total, nil
}

func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseInt(s string) (int64, error) {
	var n int64
	for _, r := range s {
		d := int64(r - '0')
		if n > (math.MaxInt64-d)/10 {
			return 0, fmt.Errorf("%w: too large", ErrAmount)
		}
		n = n*10 + d
	}
	if n > MaxMoney {
		return 0, fmt.Errorf("%w: above 21,000,000 BTC", ErrAmount)
	}
	return n, nil
}

// FormatBTC renders satoshis as a fixed 8-decimal BTC string.
func FormatBTC(sat int64) string {
	sign := ""
	if sat < 0 {
		sign = "-"
		sat = -sat
	}
	return fmt.Sprintf("%s%d.%08d", sign, sat/SatPerBTC, sat%SatPerBTC)
}
