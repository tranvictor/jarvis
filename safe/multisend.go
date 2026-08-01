package safe

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
)

// multiSendCandidate is one known MultiSendCallOnly deployment.
type multiSendCandidate struct {
	Address string
	Label   string
}

// Safe's libraries are deployed deterministically, so the same address holds
// on almost every chain for a given release. Two variants exist per release:
// the "canonical" CREATE2 deployment and an "eip155" one used on chains that
// require replay-protected deployment transactions — hence a candidate list
// rather than a single address per version.
//
// MultiSendCallOnly (not plain MultiSend) is what jarvis batches through: it
// reverts on any sub-call with operation != 0, which means a malicious or
// malformed batch cannot smuggle a DELEGATECALL into the Safe's context. The
// Safe transaction-builder format has no per-entry operation field anyway, so
// nothing is lost.
var (
	multiSendCallOnly141 = []multiSendCandidate{
		{"0x9641d764fc13c8B624c04430C7356C1C7C8102e2", "MultiSendCallOnly 1.4.1 (canonical)"},
	}
	multiSendCallOnly130 = []multiSendCandidate{
		{"0x40A2aCCbd92BCA938b02010E17A5b8929b49130D", "MultiSendCallOnly 1.3.0 (canonical)"},
		{"0xA1dabEF33b3B82c7814B6D82A79e50F4AC44102B", "MultiSendCallOnly 1.3.0 (eip155)"},
	}
)

// multiSendCandidatesFor orders the candidate deployments to probe for a Safe
// reporting the given VERSION(). The non-matching release is appended rather
// than dropped: a Safe's own version doesn't constrain which library releases
// exist on its chain, and every MultiSendCallOnly release is a standalone
// stateless library that works with any Safe version.
func multiSendCandidatesFor(safeVersion string) []multiSendCandidate {
	if strings.HasPrefix(strings.TrimSpace(safeVersion), "1.4") {
		return append(append([]multiSendCandidate{}, multiSendCallOnly141...), multiSendCallOnly130...)
	}
	return append(append([]multiSendCandidate{}, multiSendCallOnly130...), multiSendCallOnly141...)
}

// ResolveMultiSendCallOnly determines which MultiSendCallOnly contract to
// delegatecall for a batched SafeTx. It returns the address plus a
// human-readable label for the confirmation screen.
//
// An explicit override always wins and is used verbatim — jarvis only checks
// that it looks like an address, since an operator pointing at their own
// deployment is a legitimate reason to bypass the built-in list. Otherwise
// candidates are probed for on-chain code and the first live one is used; if
// none has code, this fails rather than guessing, because a delegatecall to a
// codeless address would make the whole batch a silent no-op.
func ResolveMultiSendCallOnly(
	s *SafeContract, network networks.Network, override string,
) (common.Address, string, error) {
	if o := strings.TrimSpace(override); o != "" {
		if !common.IsHexAddress(o) {
			return common.Address{}, "", fmt.Errorf(
				"--multisend-address %q is not a valid address", override,
			)
		}
		return common.HexToAddress(o), "user-supplied via --multisend-address", nil
	}

	// A Safe whose VERSION() can't be read is not fatal: it only changes the
	// probe order, and every candidate is verified on-chain anyway.
	safeVersion, _ := s.Version()
	candidates := multiSendCandidatesFor(safeVersion)

	tried := make([]string, 0, len(candidates))
	for _, c := range candidates {
		tried = append(tried, c.Address)
		isContract, err := util.IsContract(c.Address, network)
		if err != nil || !isContract {
			continue
		}
		return common.HexToAddress(c.Address), c.Label + ", verified on-chain", nil
	}

	return common.Address{}, "", fmt.Errorf(
		"couldn't locate MultiSendCallOnly on chain %d (tried %s); "+
			"pass --multisend-address <addr> explicitly",
		network.GetChainID(), strings.Join(tried, ", "),
	)
}
