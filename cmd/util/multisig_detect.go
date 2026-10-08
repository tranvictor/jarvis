package util

import (
	"fmt"
	"strings"

	"github.com/tranvictor/jarvis/msig"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/util/cache"
)

// MultisigType is the kind of on-chain multisig backing a given address.
// It's stored on TxContext so dispatching commands (jarvis msig info /
// approve / execute / ...) can pick the right code path without the user
// having to remember whether their treasury is Safe or Classic.
type MultisigType string

const (
	// MultisigUnknown means we couldn't conclusively probe the address.
	// Callers should treat this as "not a multisig" and surface a clear
	// error to the user.
	MultisigUnknown MultisigType = ""
	// MultisigSafe is a Gnosis Safe (any v1.x release). Discriminated by
	// the presence of an on-chain domainSeparator() function — this is
	// the EIP-712 anchor that classic Gnosis Multisig does not have.
	MultisigSafe MultisigType = "safe"
	// MultisigClassic is a legacy Gnosis Multisig (the gnosis.io one),
	// discriminated by getOwners() returning successfully when Safe
	// detection has already failed.
	MultisigClassic MultisigType = "classic"
)

// DetectMultisigType identifies whether address is a Gnosis Safe, a
// Classic Gnosis Multisig, or neither — caching the result on disk so
// repeated commands against the same multisig don't pay the RPC roundtrip.
//
// Detection order matters: Safe is checked first via domainSeparator(),
// which is unique to Safe (Classic does not implement EIP-712 typed
// data). Only if Safe detection fails do we fall back to a Classic probe.
//
// Cache invalidation: this is keyed by (chainID, lowercased address) and
// is treated as immutable for the lifetime of the on-disk cache. If a
// user ever migrates an address from one multisig type to another (which
// would require redeploying at the same address — practically impossible),
// they can clear the cache via `jarvis cache clear`.
func DetectMultisigType(network jarvisnetworks.Network, address string) (MultisigType, error) {
	if address == "" {
		return MultisigUnknown, fmt.Errorf("empty address")
	}
	cacheKey := fmt.Sprintf(
		"%d_%s_msig_type",
		network.GetChainID(), strings.ToLower(address),
	)
	if v, ok := cache.GetCache(cacheKey); ok && v != "" {
		switch MultisigType(v) {
		case MultisigSafe, MultisigClassic:
			return MultisigType(v), nil
		}
	}

	var probeErr error
	remember := func(err error) {
		if err != nil && probeErr == nil {
			probeErr = err
		}
	}

	if sc, err := safe.NewSafeContract(address, network); err != nil {
		remember(err)
	} else if sep, err := sc.DomainSeparator(); err == nil && sep != ([32]byte{}) {
		// A zero separator is an empty eth_call, not a Safe. Real
		// separators mix the chain id and the contract address.
		_ = cache.SetCache(cacheKey, string(MultisigSafe))
		return MultisigSafe, nil
	} else {
		remember(err)
	}

	if mc, err := msig.NewMultisigContract(address, network); err != nil {
		remember(err)
	} else if _, err := mc.NOTransactions(); err == nil {
		// transactionCount is unique to the Gnosis Classic ABI.
		_ = cache.SetCache(cacheKey, string(MultisigClassic))
		return MultisigClassic, nil
	} else if owners, oerr := mc.Owners(); oerr == nil && len(owners) > 0 {
		// Some delegations expose getOwners/required without
		// transactionCount. That is still a Classic wallet.
		if _, rerr := mc.VoteRequirement(); rerr == nil {
			_ = cache.SetCache(cacheKey, string(MultisigClassic))
			return MultisigClassic, nil
		} else {
			remember(err)
			remember(rerr)
		}
	} else {
		remember(err)
		remember(oerr)
	}

	// Safe deployments that predate domainSeparator(), or whose
	// domainSeparator() call comes back empty from a node that did not
	// execute the code, still answer getOwners + getThreshold.
	if sc, err := safe.NewSafeContract(address, network); err == nil {
		if owners, oerr := sc.Owners(); oerr == nil && len(owners) > 0 {
			if _, terr := sc.Threshold(); terr == nil {
				_ = cache.SetCache(cacheKey, string(MultisigSafe))
				return MultisigSafe, nil
			} else {
				remember(terr)
			}
		} else {
			remember(oerr)
		}
	}

	if probeErr != nil {
		return MultisigUnknown, fmt.Errorf(
			"%s is neither a Gnosis Safe nor a Gnosis Classic multisig on %s: %w",
			address, network.GetName(), probeErr,
		)
	}
	return MultisigUnknown, fmt.Errorf(
		"%s is neither a Gnosis Safe nor a Gnosis Classic multisig on %s",
		address, network.GetName(),
	)
}
