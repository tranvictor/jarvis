package util

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/accounts"
	. "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/config/state"
	"github.com/tranvictor/jarvis/msig"
	. "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/reader"
)

func AnalyzeAndShowMsigTxInfo(
	multisigContract *msig.MultisigContract,
	txid *big.Int,
	network Network,
) (fc *FunctionCall, confirmed bool, executed bool) {
	fmt.Printf("========== What the multisig will do ==========\n")
	address, value, data, executed, confirmations, err := multisigContract.TransactionInfo(txid)
	if err != nil {
		fmt.Printf("Couldn't get tx info: %s\n", err)
		return
	}

	requirement, err := multisigContract.VoteRequirement()
	if err != nil {
		fmt.Printf("Couldn't get msig requirement: %s\n", err)
		return
	}

	confirmed = len(confirmations) >= int(requirement)

	if len(data) == 0 {
		fmt.Printf(
			"Sending: %f %s to %s\n",
			BigToFloat(value, network.GetNativeTokenDecimal()),
			network.GetNativeTokenSymbol(),
			VerboseAddress(util.GetJarvisAddress(address, network)),
		)
	} else {
		destAbi, err := util.ConfigToABI(address, config.ForceERC20ABI, config.CustomABI, network)
		if err != nil {
			fmt.Printf("Couldn't get abi of destination address: %s\n", err)
			return
		}

		analyzer, err := txanalyzer.EthAnalyzer(network)
		if err != nil {
			fmt.Printf("Couldn't analyze tx: %s\n", err)
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
				fmt.Printf("This tx calls an unknown function from %s's ABI. Proceed with tx anyways.\n", address)
				return
			}

			symbol, err := util.GetERC20Symbol(address, network)
			if err != nil {
				fmt.Printf("Getting the token's symbol failed: %s. Proceed with tx anyways.\n", err)
				return
			}

			decimal, err := util.GetERC20Decimal(address, network)
			if err != nil {
				fmt.Printf("Getting the token's decimal failed: %s. Proceed with tx anyways.\n", err)
				return
			}

			switch funcCall.Method {
			case "transfer":
				isStandardERC20Call = true

				fmt.Printf(
					"From: %s\nSending: %s %s (%s)\nTo: %s\n",
					VerboseAddress(util.GetJarvisAddress(multisigContract.Address, network)),
					InfoColor(fmt.Sprintf("%f", StringToFloat(funcCall.Params[1].Value[0].Value, decimal))),
					InfoColor(symbol),
					address,
					VerboseAddress(util.GetJarvisAddress(funcCall.Params[0].Value[0].Value, network)),
				)
			case "transferFrom":
				isStandardERC20Call = true

				fmt.Printf(
					"From: %s\nSending: %f %s (%s)\nTo: %s\n",
					VerboseAddress(util.GetJarvisAddress(funcCall.Params[0].Value[0].Value, network)),
					StringToFloat(funcCall.Params[2].Value[0].Value, decimal),
					symbol,
					address,
					VerboseAddress(util.GetJarvisAddress(funcCall.Params[1].Value[0].Value, network)),
				)
			case "approve":
				isStandardERC20Call = true
				fmt.Printf(
					"Approving %s to spend upto: %f %s (%s) from the multisig\n",
					VerboseAddress(util.GetJarvisAddress(funcCall.Params[0].Value[0].Value, network)),
					StringToFloat(funcCall.Params[1].Value[0].Value, decimal),
					symbol,
					address,
				)
			}
		}

		if !isStandardERC20Call {
			fmt.Printf("Calling on %s:\n", VerboseAddress(util.GetJarvisAddress(address, network)))
			fc = util.AnalyzeMethodCallAndPrint(
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

	fmt.Printf("===============================================\n\n")
	fmt.Printf("========== Multisig transaction status ========\n")
	fmt.Printf("Executed: %t\n", executed)
	fmt.Printf("Confirmed: %t\n", confirmed)
	fmt.Printf("Confirmations (among current owners):\n")
	for i, c := range confirmations {
		_, name, err := util.GetMatchingAddress(c)
		if err != nil {
			fmt.Printf("%d. %s (Unknown)\n", i+1, c)
		} else {
			fmt.Printf("%d. %s (%s)\n", i+1, c, name)
		}
	}

	return
}

type PostProcessFunc func(fc *FunctionCall) error

func ScanForTxs(para string) (nwks []string, addresses []string) {
	networkNames := GetSupportedNetworkNames()
	regexStr := strings.Join(networkNames, "|")
	regexStr = fmt.Sprintf(
		"(?i)(?:(?P<network>%s)(?:.{0,}?))?(?P<address>(?:0x)?(?:[0-9a-fA-F]{64}))",
		regexStr,
	)

	re := regexp.MustCompile(regexStr)

	// Find all matches
	matches := re.FindAllStringSubmatch(para, -1)

	for _, match := range matches {
		nwks = append(nwks, strings.ToLower(match[1]))
		addresses = append(addresses, match[2])
	}

	return
}

func HandleApproveOrRevokeOrExecuteMsig(
	method string,
	cmd *cobra.Command,
	args []string,
	postProcess PostProcessFunc,
) {
	reader, err := util.EthReader(config.Network())
	if err != nil {
		fmt.Printf("Couldn't connect to blockchain.\n")
		return
	}

	analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

	var txid *big.Int

	if config.Tx == "" {
		nwks, txs := ScanForTxs(args[1])
		if len(txs) == 0 {
			txid, err = util.ParamToBigInt(args[1])
			if err != nil {
				fmt.Printf("Invalid second param. It must be either init tx hash or tx id.\n")
				return
			}
		} else {
			config.Tx = txs[0]
			if nwks[0] != "" {
				if err = config.SetNetwork(nwks[0]); err != nil {
					fmt.Printf("Not supported network: %s\n", err)
					return
				}
			}
		}
	}

	if txid == nil {
		if state.TxInfo == nil {
			txinfo, err := reader.TxInfoFromHash(config.Tx)
			if err != nil {
				fmt.Printf("Couldn't get tx info from the blockchain: %s\n", err)
				return
			}
			state.TxInfo = &txinfo
		}
		if state.TxInfo.Receipt == nil {
			fmt.Printf("Can't get receipt of the init tx. That tx might still be pending.\n")
			return
		}
		for _, l := range state.TxInfo.Receipt.Logs {
			if strings.ToLower(l.Address.Hex()) == strings.ToLower(config.To) &&
				l.Topics[0].Hex() == "0xc0ba8fe4b176c1714197d43b9cc6bcf797a4a7461c5fe8d0ef6e184ae7601e51" {

				txid = l.Topics[1].Big()
				break
			}
		}
		if txid == nil {
			fmt.Printf(
				"The provided tx hash is not a gnosis multisig init tx or with a different multisig.\n",
			)
			return
		}
	}

	multisigContract, err := msig.NewMultisigContract(
		config.To,
		config.Network(),
	)
	if err != nil {
		fmt.Printf("Couldn't interact with the contract: %s\n", err)
		return
	}

	fc, _, executed := AnalyzeAndShowMsigTxInfo(multisigContract, txid, config.Network())

	if postProcess != nil && postProcess(fc) != nil {
		return
	}

	if executed {
		return
	}
	// TODO: support multiple txs?

	a, err := util.GetABI(config.To, config.Network())
	if err != nil {
		fmt.Printf("Couldn't get the ABI for %s: %s\n", config.To, err)
		return
	}

	data, err := a.Pack(method, txid)
	if err != nil {
		fmt.Printf("Couldn't pack data: %s\n", err)
		return
	}

	// var GasLimit uint64
	if config.GasLimit == 0 {
		config.GasLimit, err = reader.EstimateExactGas(
			config.From,
			config.To,
			0,
			config.Value,
			data,
		)
		if err != nil {
			fmt.Printf("Couldn't estimate gas limit: %s\n", err)
			return
		}
	}

	tx := BuildExactTx(
		config.TxType,
		config.Nonce,
		config.To,
		config.Value,
		config.GasLimit+config.ExtraGasLimit,
		config.GasPrice+config.ExtraGasPrice,
		config.TipGas,
		data,
		config.Network().GetChainID(),
	)

	err = PromptTxConfirmation(
		analyzer,
		util.GetJarvisAddress(config.From, config.Network()),
		tx,
		nil,
		config.Network(),
	)
	if err != nil {
		fmt.Printf("Aborted!\n")
		return
	}

	fmt.Printf("== Unlock your wallet and sign now...\n")
	account, err := accounts.UnlockAccount(config.FromAcc)
	if err != nil {
		fmt.Printf("Failed: %s\n", err)
		return
	}

	signedAddr, signedTx, err := account.SignTx(
		tx,
		big.NewInt(int64(config.Network().GetChainID())),
	)
	if err != nil {
		fmt.Printf("Signing tx failed: %s\n", err)
		return
	}
	if signedAddr.Cmp(HexToAddress(config.FromAcc.Address)) != 0 {
		fmt.Printf(
			"Signed from wrong address. You could use wrong hw or passphrase. Expected wallet: %s, signed wallet: %s\n",
			config.FromAcc.Address,
			signedAddr.Hex(),
		)
		return
	}

	broadcaster, err := util.EthBroadcaster(config.Network())
	if err != nil {
		fmt.Printf("Signing tx failed: %s\n", err)
		return
	}

	_, broadcasted, err := broadcaster.BroadcastTx(signedTx)
	if config.DontWaitToBeMined {
		util.DisplayBroadcastedTx(
			signedTx, broadcasted, err, config.Network(),
		)
	} else {
		util.DisplayWaitAnalyze(
			reader, analyzer, signedTx, broadcasted, err, config.Network(),
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

func (s *signedTxResultJSON) Write(filepath string) {
	data, _ := json.MarshalIndent(s, "", "  ")
	err := ioutil.WriteFile(filepath, data, 0644)
	if err != nil {
		fmt.Printf("Writing to json file failed: %s\n", err)
	}
}

func HandlePostSign(
	signedTx *types.Transaction,
	reader *reader.EthReader,
	analyzer *txanalyzer.TxAnalyzer,
	a *abi.ABI,
) (broadcasted bool, err error) {
	signedData, err := rlp.EncodeToBytes(signedTx)
	if err != nil {
		fmt.Printf("Couldn't encode the signed tx: %s", err)
		return false, fmt.Errorf("Couldn't encode the signed tx: %w", err)
	}
	signedHex := hexutil.Encode(signedData)

	signerHex, err := GetSignerAddressFromTx(
		signedTx,
		big.NewInt(int64(config.Network().GetChainID())),
	)
	if err != nil {
		return false, fmt.Errorf("Couldn't derive sender address from signed tx: %w", err)
	}

	resultJSON := signedTxResultJSON{
		Tx:            signedTx,
		TxHash:        signedTx.Hash().Hex(),
		SenderAddress: signerHex.Hex(),
		SignedHex:     signedHex,
	}
	if config.JSONOutputFile != "" {
		defer resultJSON.Write(config.JSONOutputFile)
	}

	broadcaster, err := util.EthBroadcaster(config.Network())
	if err != nil {
		return false, err
	}

	if config.DontBroadcast {
		fmt.Printf("Signed tx: %s\n", signedHex)
		return false, nil
	}

	if !config.RetryBroadcast {
		_, broadcasted, err := broadcaster.BroadcastTx(signedTx)
		if config.DontWaitToBeMined {
			util.DisplayBroadcastedTx(
				signedTx, broadcasted, err, config.Network(),
			)
			return broadcasted, err
		}

		util.DisplayWaitAnalyze(
			reader, analyzer, signedTx, broadcasted, err, config.Network(),
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
					fmt.Printf("Couldn't broadcast tx: %s. Retry in a while.\n", err)
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	select {
	case <-broadcastedCh:
		if config.DontWaitToBeMined {
			util.DisplayBroadcastedTx(
				signedTx, broadcasted, err, config.Network(),
			)
			return broadcasted, err
		}

		util.DisplayWaitAnalyze(
			reader, analyzer, signedTx, broadcasted, err, config.Network(),
			a, nil, config.DegenMode,
		)
		return broadcasted, err
	}
}

func StringParamToBytes(data string) []byte {
	if data == "" {
		return []byte{}
	}

	if strings.HasPrefix(data, "0x") {
		dataBytes, err := hex.DecodeString(data[2:])
		if err != nil {
			fmt.Printf("Couldn't decode data: %s. Hex data must start with 0x and be a valid hex string. Ignore this param.\n", err)
			return []byte{}
		}
		return dataBytes
	}

	return []byte(data)
}
