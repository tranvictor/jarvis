package cmd

import (
	"fmt"

	ethcommon "github.com/ethereum/go-ethereum/common"

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
		cmdutil.PrintOwnerList(appUI, owners, config.Network())
	}
}

func safeCard(stx *safe.SafeTx, hash [32]byte, tc *cmdutil.TxContext, opt cmdutil.SafeCardOptions) *cmdutil.SigningCard {
	opt.SafeAddr = tc.SafeAddress()
	return cmdutil.BuildSafeSigningCard(stx, hash, config.Network(), tc.Resolver, tc.Analyzer, opt)
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
		appUI.Info("  %d. %s %s", i+1, cmdutil.SafeOwnerSigTag(s), appUI.Style(util.StyledAddress(jarvisAddr)))
	}
}

func safeHashArg(hash [32]byte) string {
	return "0x" + ethcommon.Bytes2Hex(hash[:])
}

func msigCmdLine(verb, addr, ident string) string {
	return fmt.Sprintf("  jarvis msig %s %s %s%s", verb, addr, ident, networkFlag())
}

func printMsigCmd(verb, addr, ident string) {
	appUI.Info("%s", msigCmdLine(verb, addr, ident))
}

func printSafeApproveExecuteHints(addr string, hash [32]byte) {
	ident := safeHashArg(hash)
	appUI.Info("Other owners can approve with:")
	printMsigCmd("approve", addr, ident)
	appUI.Info("Once threshold is met, anyone can execute with:")
	printMsigCmd("execute", addr, ident)
}

func printSafeFileHints(addr string) {
	ident := "--safe-tx-file " + safeTxFile
	appUI.Info("Share the file with other owners; each can run:")
	printMsigCmd("approve", addr, ident)
	appUI.Info("Once threshold is met, any owner can run:")
	printMsigCmd("execute", addr, ident)
}

func printSafeProposalMeta(hash [32]byte) {
	appUI.Info("network: %s (chain %d)", config.Network().GetName(), config.Network().GetChainID())
	appUI.Info("safeTxHash: %s", safeHashArg(hash))
}

func printExecuteLater(addr string, hash [32]byte) {
	ident := safeHashArg(hash)
	if safeTxFile != "" {
		ident = "--safe-tx-file " + safeTxFile
	}
	printMsigCmd("execute", addr, ident)
}
