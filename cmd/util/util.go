package util

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/accounts"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/reader"
)

// AnalyzeAndShowMsigTxInfo fetches a multisig transaction by ID, decodes and
// displays its intent, confirmation status, and list of confirmers.
func AnalyzeAndShowMsigTxInfo(
	u ui.UI,
	multisigContract *msig.MultisigContract,
	txid *big.Int,
	network jarvisnetworks.Network,
) (fc *jarviscommon.FunctionCall, confirmed bool, executed bool) {
	u.Section("What the multisig will do")
	address, value, data, executed, confirmations, err := multisigContract.TransactionInfo(txid)
	if err != nil {
		u.Error("Couldn't get tx info: %s", err)
		return
	}

	requirement, err := multisigContract.VoteRequirement()
	if err != nil {
		u.Error("Couldn't get msig requirement: %s", err)
		return
	}

	confirmed = len(confirmations) >= int(requirement)

	if len(data) == 0 {
		u.Info(
			"Sending: %f %s to %s",
			jarviscommon.BigToFloat(value, network.GetNativeTokenDecimal()),
			network.GetNativeTokenSymbol(),
			jarviscommon.VerboseAddress(util.GetJarvisAddress(address, network)),
		)
	} else {
		destAbi, err := util.ConfigToABI(address, config.ForceERC20ABI, config.CustomABI, network)
		if err != nil {
			u.Error("Couldn't get abi of destination address: %s", err)
			return
		}

		analyzer, err := txanalyzer.EthAnalyzer(network)
		if err != nil {
			u.Error("Couldn't analyze tx: %s", err)
			return
		}

		var isStandardERC20Call bool

		if util.IsERC20ABI(destAbi) {
			funcCall := analyzer.AnalyzeFunctionCallRecursively(
				util.GetABI,
				value,
				address,
				data,
				map[string]*abi.ABI{
					strings.ToLower(address): destAbi,
				},
			)
			if funcCall.Error != "" {
				u.Error("This tx calls an unknown function from %s's ABI. Proceed with tx anyways.", address)
				return
			}

			symbol, err := util.GetERC20Symbol(address, network)
			if err != nil {
				u.Error("Getting the token's symbol failed: %s. Proceed with tx anyways.", err)
				return
			}

			decimal, err := util.GetERC20Decimal(address, network)
			if err != nil {
				u.Error("Getting the token's decimal failed: %s. Proceed with tx anyways.", err)
				return
			}

			switch funcCall.Method {
			case "transfer":
				isStandardERC20Call = true
				u.Info(
					"from: %s\nSending: %s %s (%s)\nto: %s",
					jarviscommon.VerboseAddress(util.GetJarvisAddress(multisigContract.Address, network)),
					jarviscommon.InfoColor(fmt.Sprintf("%f", jarviscommon.StringToFloat(funcCall.Params[1].Values[0].Value, decimal))),
					jarviscommon.InfoColor(symbol),
					address,
					jarviscommon.VerboseAddress(util.GetJarvisAddress(funcCall.Params[0].Values[0].Value, network)),
				)
			case "transferFrom":
				isStandardERC20Call = true
				u.Info(
					"from: %s\nSending: %f %s (%s)\nto: %s",
					jarviscommon.VerboseAddress(util.GetJarvisAddress(funcCall.Params[0].Values[0].Value, network)),
					jarviscommon.StringToFloat(funcCall.Params[2].Values[0].Value, decimal),
					symbol,
					address,
					jarviscommon.VerboseAddress(util.GetJarvisAddress(funcCall.Params[1].Values[0].Value, network)),
				)
			case "approve":
				isStandardERC20Call = true
				u.Info(
					"approving %s to spend upto: %f %s (%s) from the multisig",
					jarviscommon.VerboseAddress(util.GetJarvisAddress(funcCall.Params[0].Values[0].Value, network)),
					jarviscommon.StringToFloat(funcCall.Params[1].Values[0].Value, decimal),
					symbol,
					address,
				)
			}
		}

		if !isStandardERC20Call {
			u.Info("Calling on %s:", jarviscommon.VerboseAddress(util.GetJarvisAddress(address, network)))
			fc = util.AnalyzeMethodCallAndPrint(
				u,
				analyzer,
				value,
				address,
				data,
				map[string]*abi.ABI{
					strings.ToLower(address): destAbi,
				},
				network,
			)
		}
	}

	u.Section("Multisig transaction status")
	u.Info("Executed: %t", executed)
	u.Info("Confirmed: %t", confirmed)
	u.Info("Confirmations (among current owners):")
	for i, c := range confirmations {
		_, name, err := util.GetMatchingAddress(c)
		if err != nil {
			u.Info("%d. %s (Unknown)", i+1, c)
		} else {
			u.Info("%d. %s (%s)", i+1, c, name)
		}
	}

	return
}

// PostProcessFunc is a callback called with the decoded function call after
// displaying a multisig transaction. Return an error to abort the flow.
type PostProcessFunc func(fc *jarviscommon.FunctionCall) error

// ScanForTxs scans para for network-prefixed or bare transaction hashes.
func ScanForTxs(para string) (nwks []string, addresses []string) {
	networkNames := jarvisnetworks.GetSupportedNetworkNames()
	regexStr := strings.Join(networkNames, "|")
	regexStr = fmt.Sprintf(
		"(?i)(?:(?P<network>%s)(?:.{0,}?))?(?P<address>(?:0x)?(?:[0-9a-fA-F]{64}))",
		regexStr,
	)

	re := regexp.MustCompile(regexStr)
	for _, match := range re.FindAllStringSubmatch(para, -1) {
		nwks = append(nwks, strings.ToLower(match[1]))
		addresses = append(addresses, match[2])
	}
	return
}

// HandleApproveOrRevokeOrExecuteMsig handles the confirm / revoke / execute
// flow for a Gnosis multisig transaction.
func HandleApproveOrRevokeOrExecuteMsig(
	u ui.UI,
	method string,
	cmd *cobra.Command,
	args []string,
	postProcess PostProcessFunc,
) {
	tc, _ := TxContextFrom(cmd)

	reader, err := util.EthReader(config.Network())
	if err != nil {
		u.Error("Couldn't connect to blockchain.")
		return
	}

	analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

	var txid *big.Int
	var txInfo *jarviscommon.TxInfo

	if config.Tx == "" {
		nwks, txs := ScanForTxs(args[1])
		if len(txs) == 0 {
			txid, err = util.ParamToBigInt(args[1])
			if err != nil {
				u.Error("Invalid second param. It must be either init tx hash or tx id.")
				return
			}
		} else {
			config.Tx = txs[0]
			if nwks[0] != "" {
				if err = config.SetNetwork(nwks[0]); err != nil {
					u.Error("Not supported network: %s", err)
					return
				}
			}
		}
	}

	if txid == nil {
		txInfo = tc.TxInfo
		if txInfo == nil {
			txinfo, err := reader.TxInfoFromHash(config.Tx)
			if err != nil {
				u.Error("Couldn't get tx info from the blockchain: %s", err)
				return
			}
			txInfo = &txinfo
		}
		if txInfo.Receipt == nil {
			u.Error("Can't get receipt of the init tx. That tx might still be pending.")
			return
		}
		for _, l := range txInfo.Receipt.Logs {
			if strings.EqualFold(l.Address.Hex(), tc.To) &&
				l.Topics[0].Hex() == "0xc0ba8fe4b176c1714197d43b9cc6bcf797a4a7461c5fe8d0ef6e184ae7601e51" {
				txid = l.Topics[1].Big()
				break
			}
		}
		if txid == nil {
			u.Error("The provided tx hash is not a gnosis multisig init tx or with a different multisig.")
			return
		}
	}

	multisigContract, err := msig.NewMultisigContract(tc.To, config.Network())
	if err != nil {
		u.Error("Couldn't interact with the contract: %s", err)
		return
	}

	fc, _, executed := AnalyzeAndShowMsigTxInfo(u, multisigContract, txid, config.Network())

	if postProcess != nil && postProcess(fc) != nil {
		return
	}

	if executed {
		return
	}

	a, err := util.GetABI(tc.To, config.Network())
	if err != nil {
		u.Error("Couldn't get the ABI for %s: %s", tc.To, err)
		return
	}

	data, err := a.Pack(method, txid)
	if err != nil {
		u.Error("Couldn't pack data: %s", err)
		return
	}

	if config.GasLimit == 0 {
		config.GasLimit, err = reader.EstimateExactGas(tc.From, tc.To, 0, tc.Value, data)
		if err != nil {
			u.Error("Couldn't estimate gas limit: %s", err)
			return
		}
	}

	tx := jarviscommon.BuildExactTx(
		tc.TxType,
		tc.Nonce,
		tc.To,
		tc.Value,
		config.GasLimit+config.ExtraGasLimit,
		tc.GasPrice+config.ExtraGasPrice,
		tc.TipGas,
		data,
		config.Network().GetChainID(),
	)

	err = PromptTxConfirmation(
		u,
		analyzer,
		util.GetJarvisAddress(tc.From, config.Network()),
		tx,
		nil,
		config.Network(),
	)
	if err != nil {
		u.Error("Aborted!")
		return
	}

	u.Info("Unlock your wallet and sign now...")
	account, err := accounts.UnlockAccount(tc.FromAcc)
	if err != nil {
		u.Error("Failed: %s", err)
		return
	}

	signedAddr, signedTx, err := account.SignTx(
		tx,
		big.NewInt(int64(config.Network().GetChainID())),
	)
	if err != nil {
		u.Error("Signing tx failed: %s", err)
		return
	}
	if signedAddr.Cmp(jarviscommon.HexToAddress(tc.FromAcc.Address)) != 0 {
		u.Error(
			"Signed from wrong address. You could use wrong hw or passphrase. Expected wallet: %s, signed wallet: %s",
			tc.FromAcc.Address,
			signedAddr.Hex(),
		)
		return
	}

	broadcaster, err := util.EthBroadcaster(config.Network())
	if err != nil {
		u.Error("Couldn't create broadcaster: %s", err)
		return
	}

	_, broadcasted, err := broadcaster.BroadcastTx(signedTx)
	if config.DontWaitToBeMined {
		util.DisplayBroadcastedTx(u, signedTx, broadcasted, err, config.Network())
	} else {
		util.DisplayWaitAnalyze(
			u, reader, analyzer, signedTx, broadcasted, err, config.Network(),
			a, nil, config.DegenMode,
		)
	}
}

type signedTxResultJSON struct {
	Tx            *types.Transaction `json:"transaction"`
	TxHash        string             `json:"txHash"`
	SenderAddress string             `json:"senderAddress"`
	SignedHex     string             `json:"signedHex"`
}

func (s *signedTxResultJSON) Write(u ui.UI, filepath string) {
	data, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		u.Error("Writing to json file failed: %s", err)
	}
}

// HandlePostSign encodes the signed transaction, optionally writes JSON output,
// and broadcasts (with optional retry) and/or waits for mining.
func HandlePostSign(
	u ui.UI,
	signedTx *types.Transaction,
	reader *reader.EthReader,
	analyzer *txanalyzer.TxAnalyzer,
	a *abi.ABI,
) (broadcasted bool, err error) {
	signedData, err := rlp.EncodeToBytes(signedTx)
	if err != nil {
		u.Error("couldn't encode the signed tx: %s", err)
		return false, fmt.Errorf("couldn't encode the signed tx: %w", err)
	}
	signedHex := hexutil.Encode(signedData)

	signerHex, err := jarviscommon.GetSignerAddressFromTx(
		signedTx,
		big.NewInt(int64(config.Network().GetChainID())),
	)
	if err != nil {
		return false, fmt.Errorf("couldn't derive sender address from signed tx: %w", err)
	}

	resultJSON := signedTxResultJSON{
		Tx:            signedTx,
		TxHash:        signedTx.Hash().Hex(),
		SenderAddress: signerHex.Hex(),
		SignedHex:     signedHex,
	}
	if config.JSONOutputFile != "" {
		defer resultJSON.Write(u, config.JSONOutputFile)
	}

	broadcaster, err := util.EthBroadcaster(config.Network())
	if err != nil {
		return false, err
	}

	if config.DontBroadcast {
		u.Critical("Signed tx: %s", signedHex)
		return false, nil
	}

	if !config.RetryBroadcast {
		_, broadcasted, err := broadcaster.BroadcastTx(signedTx)
		if config.DontWaitToBeMined {
			util.DisplayBroadcastedTx(u, signedTx, broadcasted, err, config.Network())
			return broadcasted, err
		}
		util.DisplayWaitAnalyze(
			u, reader, analyzer, signedTx, broadcasted, err, config.Network(),
			a, nil, config.DegenMode,
		)
		return broadcasted, err
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	quit := make(chan struct{})
	broadcastedCh := make(chan *struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				_, broadcasted, err = broadcaster.BroadcastTx(signedTx)
				if broadcasted {
					broadcastedCh <- nil
					close(quit)
				} else {
					u.Error("Couldn't broadcast tx: %s. Retry in a while.", err)
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	<-broadcastedCh
	if config.DontWaitToBeMined {
		util.DisplayBroadcastedTx(u, signedTx, broadcasted, err, config.Network())
		return broadcasted, err
	}

	util.DisplayWaitAnalyze(
		u, reader, analyzer, signedTx, broadcasted, err, config.Network(),
		a, nil, config.DegenMode,
	)
	return broadcasted, err
}

// StringParamToBytes converts a hex-prefixed or raw string to bytes.
func StringParamToBytes(data string) []byte {
	if data == "" {
		return []byte{}
	}
	if strings.HasPrefix(data, "0x") {
		dataBytes, err := hex.DecodeString(data[2:])
		if err != nil {
			return []byte{}
		}
		return dataBytes
	}
	return []byte(data)
}
