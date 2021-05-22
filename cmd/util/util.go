package util

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/accounts"
	. "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util"
)

func AnalyzeAndShowMsigTxInfo(multisigContract *msig.MultisigContract, txid *big.Int) (fc *FunctionCall) {
	address, value, data, executed, confirmations, err := multisigContract.TransactionInfo(txid)
	if err != nil {
		fmt.Printf("Jarvis: Can't get tx info: %s\n", err)
		return
	}
	fmt.Printf(
		"Sending: %f ETH to %s\n",
		ethutils.BigToFloat(value, 18),
		VerboseAddress(util.GetJarvisAddress(address, config.Network)),
	)

	if len(data) != 0 {
		fmt.Printf("Calling on %s:\n", VerboseAddress(util.GetJarvisAddress(address, config.Network)))

		destAbi, err := util.ConfigToABI(address, config.ForceERC20ABI, config.CustomABI, config.Network)
		if err != nil {
			fmt.Printf("Couldn't get abi of destination address: %s\n", err)
			return
		}

		analyzer, err := txanalyzer.EthAnalyzer(config.Network)
		if err != nil {
			fmt.Printf("Couldn't analyze tx: %s\n", err)
			return
		}
		fc = util.AnalyzeMethodCallAndPrint(
			analyzer,
			value,
			address,
			data,
			map[string]*abi.ABI{
				strings.ToLower(address): destAbi,
			},
			config.Network,
		)
	}

	fmt.Printf("\nExecuted: %t\n", executed)
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

func HandleApproveOrRevokeOrExecuteMsig(method string, cmd *cobra.Command, args []string, postProcess PostProcessFunc) {
	reader, err := util.EthReader(config.Network)
	if err != nil {
		fmt.Printf("Couldn't connect to blockchain.\n")
		return
	}

	analyzer := txanalyzer.NewGenericAnalyzer(reader)

	var txid *big.Int

	if config.Tx == "" {
		txs := util.ScanForTxs(args[1])
		if len(txs) == 0 {
			txid, err = util.ParamToBigInt(args[1])
			if err != nil {
				fmt.Printf("Invalid second param. It must be either init tx hash or tx id.\n")
				return
			}
		} else {
			config.Tx = txs[0]
		}
	}

	if txid == nil {
		if config.TxInfo == nil {
			txinfo, err := reader.TxInfoFromHash(config.Tx)
			if err != nil {
				fmt.Printf("Couldn't get tx info from the blockchain: %s\n", err)
				return
			}
			config.TxInfo = &txinfo
		}
		if config.TxInfo.Receipt == nil {
			fmt.Printf("Can't get receipt of the init tx. That tx might still be pending.\n")
			return
		}
		for _, l := range config.TxInfo.Receipt.Logs {
			if strings.ToLower(l.Address.Hex()) == strings.ToLower(config.To) &&
				l.Topics[0].Hex() == "0xc0ba8fe4b176c1714197d43b9cc6bcf797a4a7461c5fe8d0ef6e184ae7601e51" {

				txid = l.Topics[1].Big()
				break
			}
		}
		if txid == nil {
			fmt.Printf("The provided tx hash is not a gnosis multisig init tx or with a different multisig.\n")
			return
		}
	}

	multisigContract, err := msig.NewMultisigContract(
		config.To,
		config.Network,
	)
	if err != nil {
		fmt.Printf("Couldn't interact with the contract: %s\n", err)
		return
	}

	fc := AnalyzeAndShowMsigTxInfo(multisigContract, txid)

	if postProcess != nil && postProcess(fc) != nil {
		return
	}
	// TODO: support multiple txs?

	a, err := util.GetABI(config.To, config.Network)
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
		config.GasLimit, err = reader.EstimateGas(config.From, config.To, config.GasPrice+config.ExtraGasPrice, config.Value, data)
		if err != nil {
			fmt.Printf("Couldn't estimate gas limit: %s\n", err)
			return
		}
	}

	tx := ethutils.BuildTx(config.Nonce, config.To, config.Value, config.GasLimit+config.ExtraGasLimit, config.GasPrice+config.ExtraGasPrice, data)

	err = util.PromptTxConfirmation(
		analyzer,
		util.GetJarvisAddress(config.From, config.Network),
		util.GetJarvisAddress(config.To, config.Network),
		tx,
		nil,
		config.Network,
	)
	if err != nil {
		fmt.Printf("Aborted!\n")
		return
	}

	fmt.Printf("== Unlock your wallet and sign now...\n")
	account, err := accounts.UnlockAccount(config.FromAcc, config.Network)
	if err != nil {
		fmt.Printf("Failed: %s\n", err)
		return
	}
	tx, broadcasted, err := account.SignTxAndBroadcast(tx)
	if config.DontWaitToBeMined {
		util.DisplayBroadcastedTx(
			tx, broadcasted, err, config.Network,
		)
	} else {
		util.DisplayWaitAnalyze(
			reader, analyzer, tx, broadcasted, err, config.Network,
			a, nil,
		)
	}
}
