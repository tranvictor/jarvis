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
	"github.com/tranvictor/jarvis/ui"
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
	sigs      []safe.OwnerSig // owners that already signed; rendered under the card
	threshold uint64          // 0 = unknown; otherwise "n of m required"
	signer    string          // EOA that will sign, if any
	prompt    string
}

// showSafeTxToConfirm displays a SafeTx as a read-only card (no prompt).
// Pass tc so we can reach the network reader, analyzer, and ABI resolver;
// pass nil to fall back to a raw-hex display.
func showSafeTxToConfirm(stx *safe.SafeTx, hash [32]byte, tc *cmdutil.TxContext) {
	cmdutil.ShowSigningCard(appUI, buildSafeSigningCard(stx, hash, tc, safeCardOptions{kind: "Safe transaction"}))
}

// buildSafeSigningCard assembles the SigningCard for a SafeTx: the SafeTx
// parameters in the order Safe wallet UIs show them (so users can sanity-check
// side by side), the calldata decoded through jarvis's analyzer pipeline, and
// the derived warnings. extraABIs matter for batches: the calls inside a
// MultiSend payload frequently target contracts the block explorer can't give
// an ABI for, and jarvis already knows their signatures because it just
// encoded them from the tx builder batch.
func buildSafeSigningCard(
	stx *safe.SafeTx,
	hash [32]byte,
	tc *cmdutil.TxContext,
	opt safeCardOptions,
) *cmdutil.SigningCard {
	network := config.Network()
	// util.GetJarvisAddress runs through util.NewEnrichedResolver, which
	// transparently fetches verified contract names (and follows proxies)
	// from the block explorer on first miss.
	toJarvis := util.GetJarvisAddress(stx.To.Hex(), network)
	isMultiSend := jarviscommon.IsMultiSendCallData(stx.Data)

	card := &cmdutil.SigningCard{
		Kind:    opt.kind,
		Network: network.GetName(),
		To:      util.StyledAddress(toJarvis),
		Prompt:  opt.prompt,
		Safe: &cmdutil.SafeCardFields{
			Operation:      operationLabel(stx.Operation),
			DelegateCall:   stx.Operation == safe.OpDelegateCall,
			MultiSend:      isMultiSend,
			SafeNonce:      stx.Nonce.String(),
			SafeTxHash:     "0x" + ethcommon.Bytes2Hex(hash[:]),
			SafeTxGas:      stx.SafeTxGas.String(),
			BaseGas:        stx.BaseGas.String(),
			GasPrice:       stx.GasPrice.String(),
			GasToken:       stx.GasToken.Hex(),
			RefundReceiver: stx.RefundReceiver.Hex(),
			Threshold:      opt.threshold,
		},
	}
	if opt.signer != "" {
		card.Signer = util.StyledAddress(util.GetJarvisAddress(opt.signer, network))
	}
	if stx.Value != nil && stx.Value.Sign() > 0 {
		card.Value = fmt.Sprintf("%s %s (%s wei)",
			jarviscommon.BigToFloatString(stx.Value, network.GetNativeTokenDecimal()),
			network.GetNativeTokenSymbol(), stx.Value.String())
	}
	for _, sig := range opt.sigs {
		card.Safe.Signatures = append(card.Safe.Signatures, signerLine(sig))
	}

	warn := cmdutil.WarningInput{
		To:           toJarvis,
		Value:        stx.Value,
		NativeSymbol: network.GetNativeTokenSymbol(),
		HasData:      len(stx.Data) > 0,
		DelegateCall: stx.Operation == safe.OpDelegateCall,
		MultiSend:    isMultiSend,
	}
	if len(stx.Data) > 0 {
		if isContract, err := util.IsContract(stx.To.Hex(), network); err == nil {
			warn.ToIsContract = isContract
		}
		fc := decodeSafeCalldata(stx, tc, opt.extraABIs)
		if fc != nil {
			warn.Call = fc
			card.Call = util.NewFunctionCallDisplay(fc)
		} else {
			card.RawData = "0x" + ethcommon.Bytes2Hex(stx.Data)
		}
	}
	card.Warnings = cmdutil.SigningWarnings(warn)
	return card
}

// decodeSafeCalldata runs the analyzer over the SafeTx payload. It mirrors
// cmd/util.AnalyzeAndShowMsigTxInfo: fetch the destination ABI through the
// resolver (honoring --custom-abi and --erc20) and let the analyzer decode
// recursively. Returns nil when no analyzer is available or no ABI could be
// found for a non-MultiSend destination.
func decodeSafeCalldata(stx *safe.SafeTx, tc *cmdutil.TxContext, extraABIs map[string]*abi.ABI) *jarviscommon.FunctionCall {
	if tc == nil || tc.Resolver == nil || tc.Analyzer == nil {
		return nil
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
		if len(customABIs) == 0 && !jarviscommon.IsMultiSendCallData(stx.Data) {
			return nil
		}
	} else if _, taken := customABIs[strings.ToLower(stx.To.Hex())]; !taken {
		customABIs[strings.ToLower(stx.To.Hex())] = destAbi
	}
	return tc.Analyzer.AnalyzeFunctionCallRecursively(
		util.GetABI, stx.Value, stx.To.Hex(), stx.Data, customABIs,
	)
}

// signerLine renders one owner signature, tagged "[on-chain]" when it was an
// approveHash rather than an off-chain signature.
func signerLine(s safe.OwnerSig) ui.StyledText {
	jarvisAddr := util.GetJarvisAddress(s.Owner.Hex(), config.Network())
	tag := "[off-chain]"
	if safe.IsOnChainApproval(s.Sig) {
		tag = "[on-chain] "
	}
	st := util.StyledAddress(jarvisAddr)
	st.Text = tag + " " + st.Text
	return st
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
		return "DELEGATECALL (1)"
	default:
		return fmt.Sprintf("UNKNOWN (%d)", op)
	}
}
