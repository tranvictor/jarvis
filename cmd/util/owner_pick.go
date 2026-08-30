package util

import (
	"errors"
	"strings"

	"github.com/tranvictor/jarvis/accounts"
	jtypes "github.com/tranvictor/jarvis/accounts/types"
)

// OwnerPickPolicy selects how PickLocalOwner behaves when more than one
// local wallet is an owner. Callers keep their existing UX by choosing
// the policy they already implemented.
type OwnerPickPolicy int

const (
	// OwnerRequireUnique errors if zero or more than one local owner wallet
	// is found. Used by Safe preprocess, send --from Safe, and Safe batch.
	OwnerRequireUnique OwnerPickPolicy = iota
	// OwnerFirstMatch returns the first matching local wallet. Used by
	// classic send and GetApproverAccountFromMsig (those paths warn).
	OwnerFirstMatch
)

var (
	// ErrNoLocalOwner means none of the contract owners is a local wallet.
	ErrNoLocalOwner = errors.New("no local owner wallet")
	// ErrMultipleLocalOwners means more than one local wallet is an owner
	// and the policy is OwnerRequireUnique.
	ErrMultipleLocalOwners = errors.New("multiple local owner wallets")
)

type accountLookup func(string) (jtypes.AccDesc, error)

// PickLocalOwner finds local wallets among owners and applies policy.
// fromFlag is unused for the scan itself: when the user passed --from,
// callers resolve that account separately (and, on Safe paths, verify
// it with IsAmongOwners). It is accepted so call sites can pass
// config.From through without a second helper.
func PickLocalOwner(owners []string, fromFlag string, policy OwnerPickPolicy) (jtypes.AccDesc, int, error) {
	return pickLocalOwner(owners, fromFlag, policy, accounts.GetAccount)
}

func pickLocalOwner(
	owners []string,
	_ string,
	policy OwnerPickPolicy,
	lookup accountLookup,
) (jtypes.AccDesc, int, error) {
	var first jtypes.AccDesc
	n := 0
	for _, owner := range owners {
		acc, err := lookup(owner)
		if err != nil {
			continue
		}
		if n == 0 {
			first = acc
		}
		n++
	}
	if n == 0 {
		return jtypes.AccDesc{}, 0, ErrNoLocalOwner
	}
	if n > 1 && policy == OwnerRequireUnique {
		return jtypes.AccDesc{}, n, ErrMultipleLocalOwners
	}
	return first, n, nil
}

// IsAmongOwners reports whether addr is in owners (case-insensitive).
func IsAmongOwners(owners []string, addr string) bool {
	for _, o := range owners {
		if strings.EqualFold(o, addr) {
			return true
		}
	}
	return false
}
