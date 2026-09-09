package util

import (
	"errors"
	"fmt"
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
	// ErrNoLocalWallet means ~/.jarvis has no wallets at all.
	ErrNoLocalWallet = errors.New("no local wallet")
	// ErrMultipleLocalWallets means more than one local wallet exists and
	// --from was not passed.
	ErrMultipleLocalWallets = errors.New("multiple local wallets")
)

type accountLookup func(string) (jtypes.AccDesc, error)

// PickLocalOwner finds local wallets among owners and applies policy.
// fromFlag is unused for the scan itself: when the user passed --from,
// callers resolve that account separately (and, on Safe init/approve,
// verify it with IsAmongOwners). Safe execute does not require the
// executor to be an owner. It is accepted so call sites can pass
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

// chooseSafeFrom picks the wallet that will pay for a Safe transaction.
//
// requireOwner is true for init/approve. It is false for execute:
// Safe.execTransaction can be sent by anyone once the signature
// threshold is met; the executor only pays gas.
func chooseSafeFrom(
	fromFlag, safeAddr string,
	owners []string,
	requireOwner bool,
	resolveFrom func(string) (jtypes.AccDesc, error),
	lookup accountLookup,
	wallets map[string]jtypes.AccDesc,
) (jtypes.AccDesc, error) {
	if fromFlag != "" {
		fromAcc, err := resolveFrom(fromFlag)
		if err != nil {
			return jtypes.AccDesc{}, err
		}
		if requireOwner && !IsAmongOwners(owners, fromAcc.Address) {
			return jtypes.AccDesc{}, fmt.Errorf("%s is not an owner of Safe %s", fromAcc.Address, safeAddr)
		}
		return fromAcc, nil
	}

	fromAcc, _, err := pickLocalOwner(owners, fromFlag, OwnerRequireUnique, lookup)
	if err == nil {
		return fromAcc, nil
	}
	if errors.Is(err, ErrMultipleLocalOwners) {
		return jtypes.AccDesc{}, fmt.Errorf(
			"you have multiple wallets that are owners of this Safe; please specify exactly one with --from",
		)
	}
	if requireOwner {
		if errors.Is(err, ErrNoLocalOwner) {
			return jtypes.AccDesc{}, fmt.Errorf(
				"you don't have any wallet which is an owner of this Safe; please run `jarvis wallet add` first",
			)
		}
		return jtypes.AccDesc{}, err
	}
	if !errors.Is(err, ErrNoLocalOwner) {
		return jtypes.AccDesc{}, err
	}

	fromAcc, err = pickUniqueLocalWallet(wallets)
	if errors.Is(err, ErrNoLocalWallet) {
		return jtypes.AccDesc{}, fmt.Errorf(
			"no local wallet to pay for execution; please run `jarvis wallet add` or pass --from",
		)
	}
	if errors.Is(err, ErrMultipleLocalWallets) {
		return jtypes.AccDesc{}, fmt.Errorf(
			"multiple local wallets; please specify the executor with --from",
		)
	}
	return fromAcc, err
}

func pickUniqueLocalWallet(all map[string]jtypes.AccDesc) (jtypes.AccDesc, error) {
	switch len(all) {
	case 0:
		return jtypes.AccDesc{}, ErrNoLocalWallet
	case 1:
		for _, acc := range all {
			return acc, nil
		}
	default:
		return jtypes.AccDesc{}, ErrMultipleLocalWallets
	}
	return jtypes.AccDesc{}, ErrNoLocalWallet
}
