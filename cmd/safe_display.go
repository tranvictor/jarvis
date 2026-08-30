package cmd

// Human-readable Safe info, confirmation, and signer printers.

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
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

// showSafeTxToConfirm displays the parameters of a SafeTx in a way that
// matches Safe wallet UIs (so users can sanity-check side-by-side) AND
// decodes the calldata into a human-readable function call using jarvis's
// standard analyzer pipeline — exactly the way `jarvis msig` shows pending
// classic-multisig transactions. Pass tc so we can reach the network reader,
// analyzer, and ABI resolver; pass nil to fall back to a raw-hex display.
func showSafeTxToConfirm(stx *safe.SafeTx, hash [32]byte, tc *cmdutil.TxContext) {
	showSafeTxToConfirmWithABIs(stx, hash, tc, nil)
}

// showSafeTxToConfirmWithABIs is showSafeTxToConfirm plus a set of extra ABIs
// keyed by lowercased address. Batch proposals need it: the calls inside a
// MultiSend payload frequently target contracts the block explorer can't give
// an ABI for, and jarvis already knows their signatures because it just
// encoded them from the tx builder batch.
func showSafeTxToConfirmWithABIs(
	stx *safe.SafeTx,
	hash [32]byte,
	tc *cmdutil.TxContext,
	extraABIs map[string]*abi.ABI,
) {
	appUI.Section("Safe transaction details")

	// util.GetJarvisAddress runs through util.NewEnrichedResolver, which
	// transparently fetches verified contract names (and follows
	// proxies) from the block explorer on first miss — no manual
	// PrefetchContractName plumbing required here or in the analyzer
	// pipeline below.
	toJarvis := util.GetJarvisAddress(stx.To.Hex(), config.Network())
	appUI.Critical("To             : %s", appUI.Style(util.StyledAddress(toJarvis)))

	if stx.Value != nil && stx.Value.Sign() > 0 {
		appUI.Critical("Value          : %f %s (%s wei)",
			jarviscommon.BigToFloat(stx.Value, config.Network().GetNativeTokenDecimal()),
			config.Network().GetNativeTokenSymbol(),
			stx.Value.String(),
		)
	} else {
		appUI.Critical("Value          : 0")
	}
	appUI.Critical("Operation      : %s", operationLabel(stx.Operation))
	if stx.Operation == safe.OpDelegateCall && jarviscommon.IsMultiSendCallData(stx.Data) {
		// The DANGEROUS label above stays: a delegatecall really does run
		// foreign code in the Safe's context. This adds the missing why, so a
		// reviewing owner can tell a routine batch from an actual red flag.
		appUI.Critical("                 ^ this is a MultiSend batch; every call it makes is listed below")
	}
	appUI.Critical("Nonce (Safe)   : %s", stx.Nonce.String())
	appUI.Critical("safeTxGas      : %s", stx.SafeTxGas.String())
	appUI.Critical("baseGas        : %s", stx.BaseGas.String())
	appUI.Critical("gasPrice       : %s", stx.GasPrice.String())
	appUI.Critical("gasToken       : %s", stx.GasToken.Hex())
	appUI.Critical("refundReceiver : %s", stx.RefundReceiver.Hex())
	appUI.Critical("safeTxHash     : 0x%s", ethcommon.Bytes2Hex(hash[:]))

	if len(stx.Data) == 0 {
		appUI.Critical("Data           : (empty)")
		return
	}

	// Decoded calldata block. We mirror cmd/util.AnalyzeAndShowMsigTxInfo:
	// fetch the destination ABI through the resolver (honoring --custom-abi
	// and --erc20), then hand off to util.AnalyzeMethodCallAndPrint which
	// prints the function name + decoded params with token-aware formatting.
	if tc == nil || tc.Resolver == nil || tc.Analyzer == nil {
		appUI.Critical("Data (%d bytes): 0x%s", len(stx.Data), ethcommon.Bytes2Hex(stx.Data))
		return
	}
	customABIs := map[string]*abi.ABI{}
	for addr, a := range extraABIs {
		customABIs[strings.ToLower(addr)] = a
	}

	destAbi, err := tc.Resolver.ConfigToABI(
		stx.To.Hex(), config.ForceERC20ABI, config.CustomABI, config.Network(),
	)
	if err != nil {
		// MultiSend / MultiSendCallOnly is unverified on many explorers, so a
		// failure here is expected for batches and must not abort the decode:
		// the analyzer has a built-in multiSend ABI to fall back on.
		appUI.Warn("Couldn't resolve ABI of destination %s: %s", stx.To.Hex(), err)
		if len(customABIs) == 0 && !jarviscommon.IsMultiSendCallData(stx.Data) {
			appUI.Critical("Data (%d bytes): 0x%s", len(stx.Data), ethcommon.Bytes2Hex(stx.Data))
			return
		}
	} else if _, taken := customABIs[strings.ToLower(stx.To.Hex())]; !taken {
		customABIs[strings.ToLower(stx.To.Hex())] = destAbi
	}

	util.AnalyzeMethodCallAndPrint(
		appUI,
		tc.Analyzer,
		stx.Value,
		stx.To.Hex(),
		stx.Data,
		customABIs,
		config.Network(),
	)
}

// showSafeSigners renders the list of owners that have already signed,
// resolving each address through the jarvis address book so names show up
// the same way `jarvis msig` displays confirmation lists. Entries produced
// by OnChainApprovalSig (v=0) are tagged "[on-chain]" so users can tell at a
// glance which owners approved via approveHash rather than off-chain signing.
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

func operationLabel(op safe.Operation) string {
	switch op {
	case safe.OpCall:
		return "CALL (0)"
	case safe.OpDelegateCall:
		return "DELEGATECALL (1) — DANGEROUS"
	default:
		return fmt.Sprintf("UNKNOWN (%d)", op)
	}
}
