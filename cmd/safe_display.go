package cmd

// Human-readable Safe info, confirmation, and signer printers.

import (
	"github.com/ethereum/go-ethereum/accounts/abi"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/util"
)

// showSafeInfo prints owner list / threshold / version / nonce so the user
// has confidence about which Safe they're operating on. Failures here are
// non-fatal — they just degrade the displayed information.
func showSafeInfo(s *safe.SafeContract) {
	appUI.Info("Safe address : %s", s.Address)
	if v, err := s.Version(); err == nil {
		appUI.Info("Safe version : %s", v)
	}
	if n, err := s.Nonce(); err == nil {
		appUI.Info("Safe nonce   : %d (next on-chain executable)", n)
	}
	if t, err := s.Threshold(); err == nil {
		appUI.Info("Threshold    : %d", t)
	}
	if owners, err := s.Owners(); err == nil {
		appUI.Info("Owners (%d):", len(owners))
		for i, o := range owners {
			jarvisAddr := util.GetJarvisAddress(o, config.Network())
			appUI.Info("  %d. %s", i+1, appUI.Style(util.StyledAddress(jarvisAddr)))
		}
	}
}

// safeCardOptions are the per-flow choices when building a Safe signing card.
type safeCardOptions struct {
	kind      string // "Safe proposal", "Safe approval", "Safe execution", "Safe transaction"
	extraABIs map[string]*abi.ABI
	sigs      []safe.OwnerSig
	threshold uint64
	signer    string
	prompt    string
}

func buildSafeSigningCard(
	stx *safe.SafeTx,
	hash [32]byte,
	tc *cmdutil.TxContext,
	opt safeCardOptions,
) *cmdutil.SigningCard {
	var resolver cmdutil.ABIResolver
	var analyzer util.TxAnalyzer
	safeAddr := ""
	if tc != nil {
		resolver = tc.Resolver
		analyzer = tc.Analyzer
		if tc.Safe != nil && tc.Safe.Address != "" {
			safeAddr = tc.Safe.Address
		} else {
			safeAddr = tc.To
		}
	}
	return cmdutil.BuildSafeSigningCard(stx, hash, config.Network(), resolver, analyzer, cmdutil.SafeCardOptions{
		Kind:      opt.kind,
		Prompt:    opt.prompt,
		Signer:    opt.signer,
		SafeAddr:  safeAddr,
		ExtraABIs: opt.extraABIs,
		Sigs:      opt.sigs,
		Threshold: opt.threshold,
	})
}

// showSafeSigners renders the list of owners that have already signed.
func showSafeSigners(label string, sigs []safe.OwnerSig) {
	if len(sigs) == 0 {
		appUI.Info("%s: (none yet)", label)
		return
	}
	appUI.Info("%s (%d):", label, len(sigs))
	for i, s := range sigs {
		jarvisAddr := util.GetJarvisAddress(s.Owner.Hex(), config.Network())
		tag := "[off-chain]"
		if safe.IsOnChainApproval(s.Sig) {
			tag = "[on-chain] "
		}
		appUI.Info("  %d. %s %s", i+1, tag, appUI.Style(util.StyledAddress(jarvisAddr)))
	}
}
