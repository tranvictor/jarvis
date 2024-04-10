package util

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/accounts"
	. "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/config/state"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util"
)

func AnalyzeAndShowMsigTxInfo(
	multisigContract *msig.MultisigContract,
	txid *big.Int,
	network networks.Network,
) (fc *FunctionCall, executed bool) {
	fmt.Printf("========== What the multisig will do ==========\n")
	address, value, data, executed, confirmations, err := multisigContract.TransactionInfo(txid)
	if err != nil {
		fmt.Printf("Couldn't get tx info: %s\n", err)
		return
	}

	if len(data) == 0 {
		fmt.Printf(
			"Sending: %f %s to %s\n",
			BigToFloat(value, network.GetNativeTokenDecimal()),
			network.GetNativeTokenSymbol(),
			VerboseAddress(util.GetJarvisAddress(address, networks.CurrentNetwork())),
		)
	} else {
		destAbi, err := util.ConfigToABI(address, config.ForceERC20ABI, config.CustomABI, networks.CurrentNetwork())
		if err != nil {
			fmt.Printf("Couldn't get abi of destination address: %s\n", err)
			return
		}

		analyzer, err := txanalyzer.EthAnalyzer(networks.CurrentNetwork())
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
					"Sending: %s %s (%s)\nFrom: %s\nTo: %s\n",
					InfoColor(fmt.Sprintf("%f", StringToFloat(funcCall.Params[1].Value[0].Value, decimal))),
					InfoColor(symbol),
					address,
					VerboseAddress(util.GetJarvisAddress(multisigContract.Address, networks.CurrentNetwork())),
					VerboseAddress(util.GetJarvisAddress(funcCall.Params[0].Value[0].Value, networks.CurrentNetwork())),
				)
			case "transferFrom":
				isStandardERC20Call = true

				fmt.Printf(
					"Sending: %f %s (%s)\nFrom: %s\nTo: %s\n",
					StringToFloat(funcCall.Params[2].Value[0].Value, decimal),
					symbol,
					address,
					VerboseAddress(util.GetJarvisAddress(funcCall.Params[0].Value[0].Value, networks.CurrentNetwork())),
					VerboseAddress(util.GetJarvisAddress(funcCall.Params[1].Value[0].Value, networks.CurrentNetwork())),
				)
			case "approve":
				isStandardERC20Call = true
				fmt.Printf(
					"Approving %s to spend upto: %f %s (%s) from the multisig\n",
					VerboseAddress(util.GetJarvisAddress(funcCall.Params[0].Value[0].Value, networks.CurrentNetwork())),
					StringToFloat(funcCall.Params[1].Value[0].Value, decimal),
					symbol,
					address,
				)
			}
		}

		if !isStandardERC20Call {
			fmt.Printf("Calling on %s:\n", VerboseAddress(util.GetJarvisAddress(address, networks.CurrentNetwork())))
			fc = util.AnalyzeMethodCallAndPrint(
				analyzer,
				value,
				address,
				data,
				map[string]*abi.ABI{
					strings.ToLower(address): destAbi,
				},
				networks.CurrentNetwork(),
			)
		}
	}

	fmt.Printf("===============================================\n\n")
	fmt.Printf("========== Multisig transaction status ========\n")
	fmt.Printf("Executed: %t\n", executed)
	fmt.Printf("Confirmations (among current owners):\n")
	for i, c := range confirmations {
		_, name, err := util.GetMatchingAddress(c)
		if err != nil {
			fmt.Printf("%d. %s (Unknown)\n", i+1, c)
		} else {
			fmt.Printf("%d. %s (%s)\n", i+1, c, name)
		}
	}

	if executed {
		fmt.Printf("This multisig is executed, you don't need to approve it anymore\n")
	}
	return
}

type PostProcessFunc func(fc *FunctionCall) error

func ScanForTxs(para string) (nwks []string, addresses []string) {
	networkNames := networks.GetSupportedNetworkNames()
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
	reader, err := util.EthReader(networks.CurrentNetwork())
	if err != nil {
		fmt.Printf("Couldn't connect to blockchain.\n")
		return
	}

	analyzer := txanalyzer.NewGenericAnalyzer(reader, networks.CurrentNetwork())

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
				networks.SetNetwork(nwks[0])
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
		networks.CurrentNetwork(),
	)
	if err != nil {
		fmt.Printf("Couldn't interact with the contract: %s\n", err)
		return
	}

	fc, executed := AnalyzeAndShowMsigTxInfo(multisigContract, txid, networks.CurrentNetwork())

	if postProcess != nil && postProcess(fc) != nil {
		return
	}

	if executed {
		return
	}
	// TODO: support multiple txs?

	a, err := util.GetABI(config.To, networks.CurrentNetwork())
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
		config.Nonce,
		config.To,
		config.Value,
		config.GasLimit+config.ExtraGasLimit,
		config.GasPrice+config.ExtraGasPrice,
		config.TipGas,
		data,
	)

	err = PromptTxConfirmation(
		analyzer,
		util.GetJarvisAddress(config.From, networks.CurrentNetwork()),
		tx,
		nil,
		networks.CurrentNetwork(),
	)
	if err != nil {
		fmt.Printf("Aborted!\n")
		return
	}

	fmt.Printf("== Unlock your wallet and sign now...\n")
	account, err := accounts.UnlockAccount(config.FromAcc, networks.CurrentNetwork())
	if err != nil {
		fmt.Printf("Failed: %s\n", err)
		return
	}
	tx, broadcasted, err := account.SignTxAndBroadcast(tx)
	if config.DontWaitToBeMined {
		util.DisplayBroadcastedTx(
			tx, broadcasted, err, networks.CurrentNetwork(),
		)
	} else {
		util.DisplayWaitAnalyze(
			reader, analyzer, tx, broadcasted, err, networks.CurrentNetwork(),
			a, nil, config.DegenMode,
		)
	}
}
