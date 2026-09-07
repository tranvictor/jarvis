package util

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
)

// decodeClassicCalldata runs the analyzer over a Classic msig inner call.
// It mirrors decodeSafeCalldata: fetch the destination ABI through the
// resolver (honoring --custom-abi and --erc20) and let the analyzer decode
// recursively. Returns nil when there is no analyzer or no data.
func decodeClassicCalldata(
	to string,
	value *big.Int,
	data []byte,
	network jarvisnetworks.Network,
	resolver ABIResolver,
	analyzer util.TxAnalyzer,
) *jarviscommon.FunctionCall {
	if analyzer == nil || len(data) == 0 {
		return nil
	}
	customABIs := map[string]*abi.ABI{}
	if resolver != nil {
		if destAbi, err := resolver.ConfigToABI(to, config.ForceERC20ABI, config.CustomABI, network); err == nil {
			customABIs[strings.ToLower(to)] = destAbi
		}
	}
	return analyzer.AnalyzeFunctionCallRecursively(util.GetABI, value, to, data, customABIs)
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
			TxID:          txid.String(),
			Multisig:      util.StyledAddress(util.GetJarvisAddress(msigAddr, network)),
			Executed:      executed,
			Confirmations: len(confirmations),
			Threshold:     uint64(threshold),
		},
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
	if len(data) > 0 {
		if isContract, err := util.IsContract(to, network); err == nil {
			warn.ToIsContract = isContract
		}
		if fc != nil && fc.Method != "" {
			card.Call = util.NewFunctionCallDisplay(fc, network)
		} else {
			card.RawData = "0x" + ethcommon.Bytes2Hex(data)
		}
	}
	card.Warnings = SigningWarnings(warn)
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
