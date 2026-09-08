package util

import (
	"math/big"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/msig"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
)

func decodeClassicCalldata(
	to string,
	value *big.Int,
	data []byte,
	network jarvisnetworks.Network,
	resolver ABIResolver,
	analyzer util.TxAnalyzer,
) *jarviscommon.FunctionCall {
	return decodeSigningCalldata(to, value, data, network, resolver, analyzer, nil)
}

func buildClassicMsigCard(
	msigAddr string,
	txid *big.Int,
	to string,
	value *big.Int,
	data []byte,
	executed bool,
	confirmations []string,
	threshold int64,
	network jarvisnetworks.Network,
	fc *jarviscommon.FunctionCall,
) *SigningCard {
	toJarvis := util.GetJarvisAddress(to, network)
	card := &SigningCard{
		Kind:    "Classic multisig transaction",
		Network: network.GetName(),
		To:      util.StyledAddress(toJarvis),
		Classic: &ClassicCardFields{
			Multisig:      util.StyledAddress(util.GetJarvisAddress(msigAddr, network)),
			Executed:      executed,
			Confirmations: len(confirmations),
			Threshold:     uint64(threshold),
		},
	}
	if txid != nil {
		card.Classic.TxID = txid.String()
	}
	if value != nil && value.Sign() > 0 {
		card.Value = jarviscommon.BigToFloatString(value, network.GetNativeTokenDecimal()) +
			" " + network.GetNativeTokenSymbol()
	}
	for _, c := range confirmations {
		card.Classic.Signatures = append(card.Classic.Signatures, util.StyledAddress(util.GetJarvisAddress(c, network)))
	}

	warn := WarningInput{
		To:             toJarvis,
		Value:          value,
		NativeSymbol:   network.GetNativeTokenSymbol(),
		NativeDecimals: network.GetNativeTokenDecimal(),
		HasData:        len(data) > 0,
		Call:           fc,
	}
	fillDestinationWarn(&warn, to, network)
	attachMultisigInnerCall(card, &warn, toJarvis, value, data, fc, network, true)
	return card
}

// AnalyzeAndShowMsigTxInfo fetches a Classic Gnosis transaction by ID, decodes
// the inner call once, and prints it as a signing card (same shape as Safe
// `msig info`). The returned FunctionCall is the decode used on the card.
func AnalyzeAndShowMsigTxInfo(
	u ui.UI,
	multisigContract *msig.MultisigContract,
	txid *big.Int,
	network jarvisnetworks.Network,
	resolver ABIResolver,
	analyzer util.TxAnalyzer,
) (fc *jarviscommon.FunctionCall, numConfirmations int, confirmed bool, executed bool, err error) {
	var address string
	var value *big.Int
	var data []byte
	var confirmations []string
	address, value, data, executed, confirmations, err = multisigContract.TransactionInfo(txid)
	if err != nil {
		u.Error("Couldn't get tx info: %s", err)
		return
	}

	requirement, err := multisigContract.VoteRequirement()
	if err != nil {
		u.Error("Couldn't get msig requirement: %s", err)
		return
	}

	numConfirmations = len(confirmations)
	confirmed = numConfirmations >= int(requirement)
	fc = decodeClassicCalldata(address, value, data, network, resolver, analyzer)
	ShowSigningCard(u, buildClassicMsigCard(
		multisigContract.Address, txid, address, value, data,
		executed, confirmations, requirement, network, fc,
	))
	return
}

// BuildClassicProposalCard is the inner Classic call a WalletConnect dApp
// (or similar) wants the msig to submit. There is no on-chain tx id yet.
func BuildClassicProposalCard(
	msigAddr, to string,
	value *big.Int,
	data []byte,
	threshold int64,
	network jarvisnetworks.Network,
	resolver ABIResolver,
	analyzer util.TxAnalyzer,
) *SigningCard {
	fc := decodeClassicCalldata(to, value, data, network, resolver, analyzer)
	return buildClassicMsigCard(msigAddr, nil, to, value, data, false, nil, threshold, network, fc)
}

// SetClassicSigningNote collapses the following EOA confirm/revoke/execute
// card: the inner Classic transaction was just shown.
func SetClassicSigningNote() {
	SetNextSigningNote(SigningNote{
		CollapseCallNote: "(Classic transaction shown above)",
	})
}

// ClassicSummaryStatus is the pending-queue label for one Classic tx id.
func ClassicSummaryStatus(confirmed int, required int64, executed bool) string {
	if executed {
		return "executed"
	}
	if required > 0 && int64(confirmed) >= required {
		return "ready to execute"
	}
	return "pending"
}
