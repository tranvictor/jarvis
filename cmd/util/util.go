package util

import (
	"encoding/hex"
	"encoding/json"
	"errors"
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
	jtypes "github.com/tranvictor/jarvis/accounts/types"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
	utilreader "github.com/tranvictor/jarvis/util/reader"
)

// AnalyzeAndShowMsigTxInfo fetches a multisig transaction by ID, decodes and
// displays its intent, confirmation status, and list of confirmers.
func AnalyzeAndShowMsigTxInfo(
	u ui.UI,
	multisigContract *msig.MultisigContract,
	txid *big.Int,
	network jarvisnetworks.Network,
	resolver ABIResolver,
	analyzer util.TxAnalyzer,
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
		destAbi, err := resolver.ConfigToABI(address, config.ForceERC20ABI, config.CustomABI, network)
		if err != nil {
			u.Error("Couldn't get abi of destination address: %s", err)
			return
		}

		var isStandardERC20Call bool

		if util.IsERC20ABI(destAbi) {
			funcCall := analyzer.AnalyzeFunctionCallRecursively(
				resolver.GetABI,
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
		_, name, err := resolver.GetMatchingAddress(c)
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

	reader := tc.Reader
	if reader == nil {
		u.Error("Couldn't connect to blockchain.")
		return
	}

	analyzer := tc.Analyzer

	var (
		err    error
		txid   *big.Int
		txInfo *jarviscommon.TxInfo
	)

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

	fc, _, executed := AnalyzeAndShowMsigTxInfo(u, multisigContract, txid, config.Network(), tc.Resolver, analyzer)

	if postProcess != nil && postProcess(fc) != nil {
		return
	}

	if executed {
		u.Warn("This transaction has already been executed. Nothing to do.")
		return
	}

	a, err := tc.Resolver.GetABI(tc.To, config.Network())
	if err != nil {
		u.Error("Couldn't get the ABI for %s: %s", tc.To, err)
		return
	}

	data, err := a.Pack(method, txid)
	if err != nil {
		u.Error("Couldn't pack data: %s", err)
		return
	}

	gasLimit := config.GasLimit
	if gasLimit == 0 {
		gasLimit, err = reader.EstimateExactGas(tc.From, tc.To, 0, tc.Value, data)
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
		gasLimit+config.ExtraGasLimit,
		tc.GasPrice+config.ExtraGasPrice,
		tc.TipGas+config.ExtraTipGas,
		data,
		config.Network().GetChainID(),
	)

	bc := tc.Broadcaster
	if bc == nil {
		u.Error("Broadcaster not available.")
		return
	}

	if broadcasted, err := SignAndBroadcast(u, tc.FromAcc, tx, nil, reader, analyzer, a, bc); err != nil && !broadcasted {
		u.Error("Failed to proceed after signing the tx: %s. Aborted.", err)
	}
}

// ErrWalletUnlock is returned by SignAndBroadcast when the wallet cannot be
// unlocked. Callers that need a specific exit code (e.g. 126) can test with
// errors.Is.
var ErrWalletUnlock = errors.New("wallet unlock failed")

// SignAndBroadcast prompts the user for confirmation, unlocks the wallet,
// signs the transaction, verifies the signer, and hands off to HandlePostSign.
func SignAndBroadcast(
	u ui.UI,
	fromAcc jtypes.AccDesc,
	tx *types.Transaction,
	customABIs map[string]*abi.ABI,
	reader utilreader.Reader,
	analyzer util.TxAnalyzer,
	a *abi.ABI,
	bc TxBroadcaster,
) (bool, error) {
	if err := PromptTxConfirmation(u, analyzer, util.GetJarvisAddress(fromAcc.Address, config.Network()), tx, customABIs, config.Network()); err != nil {
		u.Error("Aborted!")
		return false, err
	}

	u.Info("Unlock your wallet and sign now...")
	account, err := accounts.UnlockAccount(fromAcc)
	if err != nil {
		return false, fmt.Errorf("%w: %s", ErrWalletUnlock, err)
	}

	signedAddr, signedTx, err := account.SignTx(tx, big.NewInt(int64(config.Network().GetChainID())))
	if err != nil {
		return false, fmt.Errorf("couldn't sign tx: %w", err)
	}
	if signedAddr.Cmp(jarviscommon.HexToAddress(fromAcc.Address)) != 0 {
		return false, fmt.Errorf(
			"signed from wrong address. You could use wrong hw or passphrase. Expected wallet: %s, signed wallet: %s",
			fromAcc.Address,
			signedAddr.Hex(),
		)
	}

	return HandlePostSign(u, signedTx, reader, analyzer, a, bc)
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
	reader utilreader.Reader,
	analyzer util.TxAnalyzer,
	a *abi.ABI,
	broadcaster TxBroadcaster,
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
