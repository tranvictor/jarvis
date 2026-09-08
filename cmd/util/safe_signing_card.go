package util

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
)

// SafeCardOptions are the per-flow choices when building a Safe signing card.
type SafeCardOptions struct {
	Kind      string // "Safe proposal", "Safe approval", "Safe execution", "Safe transaction"
	Prompt    string
	Signer    string // EOA that will sign, if any
	ExtraABIs map[string]*abi.ABI
	Sigs      []safe.OwnerSig
	Threshold uint64 // 0 = unknown; otherwise "n of m required"
}

// BuildSafeSigningCard assembles the SigningCard for a SafeTx. extraABIs
// matter for batches: inner MultiSend calls often target contracts the
// explorer cannot name, and jarvis already knows those signatures.
func BuildSafeSigningCard(
	stx *safe.SafeTx,
	hash [32]byte,
	network jarvisnetworks.Network,
	resolver ABIResolver,
	analyzer util.TxAnalyzer,
	opt SafeCardOptions,
) *SigningCard {
	toJarvis := util.GetJarvisAddress(stx.To.Hex(), network)
	isMultiSend := jarviscommon.IsMultiSendCallData(stx.Data)

	card := &SigningCard{
		Kind:    opt.Kind,
		Network: network.GetName(),
		To:      util.StyledAddress(toJarvis),
		Prompt:  opt.Prompt,
		Safe: &SafeCardFields{
			Operation:      safeOperationLabel(stx.Operation),
			DelegateCall:   stx.Operation == safe.OpDelegateCall,
			MultiSend:      isMultiSend,
			SafeNonce:      stx.Nonce.String(),
			SafeTxHash:     "0x" + ethcommon.Bytes2Hex(hash[:]),
			SafeTxGas:      stx.SafeTxGas.String(),
			BaseGas:        stx.BaseGas.String(),
			GasPrice:       stx.GasPrice.String(),
			GasToken:       stx.GasToken.Hex(),
			RefundReceiver: stx.RefundReceiver.Hex(),
			Threshold:      opt.Threshold,
		},
	}
	if opt.Signer != "" {
		card.Signer = util.StyledAddress(util.GetJarvisAddress(opt.Signer, network))
	}
	if stx.Value != nil && stx.Value.Sign() > 0 {
		card.Value = jarviscommon.BigToFloatString(stx.Value, network.GetNativeTokenDecimal()) +
			" " + network.GetNativeTokenSymbol()
	}
	for _, sig := range opt.Sigs {
		card.Safe.Signatures = append(card.Safe.Signatures, safeSignerLine(sig, network))
	}

	warn := WarningInput{
		To:             toJarvis,
		Value:          stx.Value,
		NativeSymbol:   network.GetNativeTokenSymbol(),
		NativeDecimals: network.GetNativeTokenDecimal(),
		HasData:        len(stx.Data) > 0,
		DelegateCall:   stx.Operation == safe.OpDelegateCall,
		MultiSend:      isMultiSend,
	}
	if len(stx.Data) > 0 {
		if isContract, err := util.IsContract(stx.To.Hex(), network); err == nil {
			warn.ToIsContract = isContract
		}
		fc := decodeSafeCalldata(stx, network, resolver, analyzer, opt.ExtraABIs)
		if fc != nil {
			warn.Call = fc
			card.Call = util.NewFunctionCallDisplay(fc, network)
		} else {
			card.RawData = "0x" + ethcommon.Bytes2Hex(stx.Data)
		}
	}
	card.Warnings = SigningWarnings(warn)
	return card
}

func decodeSafeCalldata(
	stx *safe.SafeTx,
	network jarvisnetworks.Network,
	resolver ABIResolver,
	analyzer util.TxAnalyzer,
	extraABIs map[string]*abi.ABI,
) *jarviscommon.FunctionCall {
	if analyzer == nil {
		return nil
	}
	customABIs := map[string]*abi.ABI{}
	for addr, a := range extraABIs {
		customABIs[strings.ToLower(addr)] = a
	}
	if resolver != nil {
		destAbi, err := resolver.ConfigToABI(
			stx.To.Hex(), config.ForceERC20ABI, config.CustomABI, network,
		)
		if err == nil {
			if _, taken := customABIs[strings.ToLower(stx.To.Hex())]; !taken {
				customABIs[strings.ToLower(stx.To.Hex())] = destAbi
			}
		}
	}
	// Always run the analyzer, even when the explorer ABI lookup failed.
	// AnalyzeFunctionCallRecursively falls back to the standard ERC-20 ABI
	// (and MultiSend) so approve/transfer still decode instead of rendering
	// as raw bytes with a "no ABI" warning.
	return analyzer.AnalyzeFunctionCallRecursively(
		lookupABI(resolver), stx.Value, stx.To.Hex(), stx.Data, customABIs,
	)
}

func lookupABI(resolver ABIResolver) jarviscommon.ABIDatabase {
	if resolver != nil {
		return resolver.GetABI
	}
	return util.GetABI
}

func safeSignerLine(s safe.OwnerSig, network jarvisnetworks.Network) ui.StyledText {
	st := util.StyledAddress(util.GetJarvisAddress(s.Owner.Hex(), network))
	tag := "[off-chain]"
	if safe.IsOnChainApproval(s.Sig) {
		tag = "[on-chain] "
	}
	st.Text = tag + " " + st.Text
	return st
}

func safeOperationLabel(op safe.Operation) string {
	switch op {
	case safe.OpCall:
		return "CALL (0)"
	case safe.OpDelegateCall:
		return "DELEGATECALL (1)"
	default:
		return fmt.Sprintf("UNKNOWN (%d)", op)
	}
}
