package walletconnect

import (
	"fmt"

	jtypes "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
)

// AccountKind classifies a jarvis account address into one of the three
// gateway shapes the session layer knows how to build. It's the WC
// entry point equivalent of what CommonMultisigTxPreprocess already
// does for the msig command.
type AccountKind string

const (
	// KindEOA means the address resolves to a local keystore / Ledger
	// / Trezor wallet in ~/.jarvis and has no multisig contract at
	// that address on the primary chain.
	KindEOA AccountKind = "eoa"

	// KindSafe means the address is an on-chain Gnosis Safe (any
	// version) on the primary chain.
	KindSafe AccountKind = "safe"

	// KindClassic means the address is a Gnosis Classic multisig on
	// the primary chain (no domainSeparator, has getOwners /
	// NOTransactions).
	KindClassic AccountKind = "classic"
)

// Classification is the combined output of ClassifyAccount: the kind
// picked, plus the underlying jarvis AccDesc for EOA cases (so the
// command layer doesn't re-fuzzy-match the same input twice).
//
// For multisig kinds, AccDesc is left zero-valued — the caller selects
// a signing account separately based on the Safe/Classic owners and
// the --from flag.
type Classification struct {
	Kind    AccountKind
	Address string // canonical lowercased-hex
	AccDesc jtypes.AccDesc
}

// ClassifyAccount decides which gateway to build for `input` on the
// given primary chain. Resolution order:
//
//  1. Fuzzy-match input against local jarvis accounts via the existing
//     cmdutil.ResolveAccount helper (which also handles ENS). If that
//     succeeds we've got an EOA — but we still ask the detector
//     whether the resolved address is actually a multisig contract on
//     this chain, because the user may have a wallet file for a Safe
//     owner whose address happens to match the Safe itself (rare but
//     possible when re-using a seed phrase).
//  2. If the input doesn't match a local wallet, we assume it's an
//     address (or ENS name resolvable to one) backing a multisig and
//     hand off to cmdutil.DetectMultisigType.
//
// The returned Address is always lowercased-hex so CAIP-10 identifiers
// compare cleanly upstream.
func ClassifyAccount(
	resolver cmdutil.ABIResolver,
	network jarvisnetworks.Network,
	input string,
) (Classification, error) {
	if input == "" {
		return Classification{}, fmt.Errorf("walletconnect: --from is required")
	}

	// Try the "is this a local wallet?" path first. ResolveAccount
	// already handles ENS + address-book fuzzy matching so we don't
	// need to reinvent that here.
	acc, resolved, err := cmdutil.ResolveAccount(resolver, input)
	if err == nil {
		// We found a local wallet. Still probe on-chain to make sure
		// the address isn't also a multisig contract; if it is, the
		// multisig path should win because that's what the user
		// clearly means when they point WC at a multisig address.
		if mt, derr := cmdutil.DetectMultisigType(network, resolved); derr == nil {
			switch mt {
			case cmdutil.MultisigSafe:
				return Classification{
					Kind:    KindSafe,
					Address: lowerHex(resolved),
				}, nil
			case cmdutil.MultisigClassic:
				return Classification{
					Kind:    KindClassic,
					Address: lowerHex(resolved),
				}, nil
			}
		}
		return Classification{
			Kind:    KindEOA,
			Address: lowerHex(resolved),
			AccDesc: acc,
		}, nil
	}

	// Not a local wallet: probe the address as a multisig. This is
	// the same detector cmd/msig.go uses so the result is cached on
	// disk between `jarvis msig` and `jarvis wc` invocations.
	mt, derr := cmdutil.DetectMultisigType(network, input)
	if derr != nil {
		return Classification{}, fmt.Errorf(
			"walletconnect: %s is neither a local jarvis wallet "+
				"nor a recognised multisig on %s: %w",
			input, network.GetName(), derr)
	}
	switch mt {
	case cmdutil.MultisigSafe:
		return Classification{Kind: KindSafe, Address: lowerHex(input)}, nil
	case cmdutil.MultisigClassic:
		return Classification{Kind: KindClassic, Address: lowerHex(input)}, nil
	}
	return Classification{}, fmt.Errorf(
		"walletconnect: could not classify %s on %s", input, network.GetName())
}

// lowerHex normalises an address string for use inside CAIP-10
// identifiers. We don't re-validate the hex shape here because every
// caller has already funnelled through ResolveAccount or
// DetectMultisigType, both of which reject malformed input.
func lowerHex(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}
