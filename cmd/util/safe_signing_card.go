package util

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
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
	SafeAddr  string // the Safe wallet; shown on the card and as Send ... from
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
	if opt.SafeAddr != "" {
		card.Safe.Address = util.StyledAddress(util.GetJarvisAddress(opt.SafeAddr, network))
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
	fillDestinationWarn(&warn, stx.To.Hex(), network)
	fc := decodeSigningCalldata(stx.To.Hex(), stx.Value, stx.Data, network, resolver, analyzer, opt.ExtraABIs)
	attachMultisigInnerCall(card, &warn, toJarvis, stx.Value, stx.Data, fc, network, false)
	return card
}

func SafeOwnerSigTag(s safe.OwnerSig) string {
	if safe.IsOnChainApproval(s.Sig) {
		return "[on-chain] "
	}
	return "[off-chain]"
}

func safeSignerLine(s safe.OwnerSig, network jarvisnetworks.Network) ui.StyledText {
	st := util.StyledAddress(util.GetJarvisAddress(s.Owner.Hex(), network))
	st.Text = SafeOwnerSigTag(s) + " " + st.Text
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
