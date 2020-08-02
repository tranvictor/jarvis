package cmd

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/db"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/util"
)

var summaryMsigCmd = &cobra.Command{
	Use:   "summary",
	Short: "Print all txs confirmation and execution status of the multisig",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		msigAddress, err := getMsigContractFromParams(args)
		if err != nil {
			return
		}

		multisigContract, err := msig.NewMultisigContract(
			msigAddress,
			config.Network,
		)
		if err != nil {
			fmt.Printf("Couldn't interact with the contract: %s\n", err)
			return
		}

		noTxs, err := multisigContract.NOTransactions()
		if err != nil {
			fmt.Printf("Couldn't get number of transactions of the multisig: %s\n", err)
			return
		}
		fmt.Printf("Number of transactions: %d\n", noTxs)
		noExecuted := 0
		noRevoked := 0
		noConfirmed := 0
		noError := 0
		nonExecutedConfirmedIds := []int64{}
		for i := int64(0); i < noTxs; i++ {
			executed, err := multisigContract.IsExecuted(big.NewInt(i))
			if err != nil {
				fmt.Printf("%d. %s\n", i, err)
				noError++
				continue
			}
			if executed {
				fmt.Printf("%d. executed\n", i)
				noExecuted++
				noConfirmed++
				continue
			}
			confirmed, err := multisigContract.IsConfirmed(big.NewInt(i))
			if err != nil {
				fmt.Printf("%d. %s\n", i, err)
				noError++
				continue
			}
			if confirmed {
				fmt.Printf("%d. confirmed - not executed\n", i)
				nonExecutedConfirmedIds = append(nonExecutedConfirmedIds, i)
				noConfirmed++
				continue
			}
			fmt.Printf("%d. unconfirmed\n", i)
		}
		fmt.Printf("------------\n")
		fmt.Printf("Total executed txs: %d\n", noExecuted)
		fmt.Printf("Total confirmed but NOT executed txs: %d. Their ids are: %v\n", noConfirmed-noExecuted, nonExecutedConfirmedIds)
		fmt.Printf("Total revoked txs: %d\n", noRevoked)
		fmt.Printf("Total unconfirmed txs (including unknown txs): %d\n", int(noTxs)-noConfirmed-noRevoked)
	},
}

func showMsigTxInfo(multisigContract *msig.MultisigContract, txid *big.Int) {
	address, value, data, executed, confirmations, err := multisigContract.TransactionInfo(txid)
	if err != nil {
		fmt.Printf("Jarvis: Can't get tx info: %s\n", err)
		return
	}
	fmt.Printf("Sending: %f ETH to %s\n", ethutils.BigToFloat(value, 18), util.VerboseAddress(address, config.Network))
	if len(data) != 0 {
		fmt.Printf("Calling on %s:\n", util.VerboseAddress(address, config.Network))
		var destAbi *abi.ABI

		if config.ForceERC20ABI {
			destAbi, err = ethutils.GetERC20ABI()
		} else if config.CustomABI != "" {
			destAbi, err = util.ReadCustomABI(config.CustomABI, config.Network)
		} else {
			destAbi, err = util.GetABI(address, config.Network)
		}
		if err != nil {
			fmt.Printf("Couldn't get abi of destination address: %s\n", err)
			return
		}

		analyzer, err := util.EthAnalyzer(config.Network)
		if err != nil {
			fmt.Printf("Couldn't analyze tx: %s\n", err)
			return
		}
		util.AnalyzeMethodCallAndPrint(analyzer, destAbi, data, config.Network)
	}

	fmt.Printf("\nExecuted: %t\n", executed)
	fmt.Printf("Confirmations (among current owners):\n")
	for i, c := range confirmations {
		addrDesc, err := db.GetAddress(c)
		if err != nil {
			fmt.Printf("%d. %s (Unknown)\n", i+1, c)
		} else {
			fmt.Printf("%d. %s (%s)\n", i+1, c, addrDesc.Desc)
		}
	}
}

var transactionInfoMsigCmd = &cobra.Command{
	Use:   "info",
	Short: "Print all information about a multisig init",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		msigAddress, err := getMsigContractFromParams(args)
		if err != nil {
			return
		}

		multisigContract, err := msig.NewMultisigContract(
			msigAddress,
			config.Network,
		)
		if err != nil {
			fmt.Printf("Couldn't interact with the contract: %s\n", err)
			return
		}

		if len(args) < 2 {
			fmt.Printf("Jarvis: Please specify tx id in either hex or int format after the multisig address\n")
			return
		}

		idStr := strings.Trim(args[1], " ")
		txid, err := util.ParamToBigInt(idStr)
		if err != nil {
			fmt.Printf("Jarvis: Can't convert \"%s\" to int.\n", idStr)
			return
		}

		showMsigTxInfo(multisigContract, txid)
	},
}

var govInfoMsigCmd = &cobra.Command{
	Use:   "gov",
	Short: "Print goverance information of a Gnosis multisig",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		msigAddress, err := getMsigContractFromParams(args)
		if err != nil {
			return
		}

		multisigContract, err := msig.NewMultisigContract(
			msigAddress,
			config.Network,
		)
		if err != nil {
			fmt.Printf("Couldn't interact with the contract: %s\n", err)
			return
		}

		fmt.Printf("Owner list:\n")
		owners, err := multisigContract.Owners()
		if err != nil {
			fmt.Printf("Couldn't get owners of the multisig: %s\n", err)
			return
		}
		for i, owner := range owners {
			addrDesc, err := db.GetAddress(owner)
			if err != nil {
				fmt.Printf("%d. %s (Unknown)\n", i+1, owner)
			} else {
				fmt.Printf("%d. %s (%s)\n", i+1, owner, addrDesc.Desc)
			}
		}
		voteRequirement, err := multisigContract.VoteRequirement()
		if err != nil {
			fmt.Printf("Couldn't get vote requiremnts of the multisig: %s\n", err)
			return
		}
		fmt.Printf("Vote requirement: %d/%d\n", voteRequirement, len(owners))
		noTxs, err := multisigContract.NOTransactions()
		if err != nil {
			fmt.Printf("Couldn't get number of transactions of the multisig: %s\n", err)
			return
		}
		fmt.Printf("Number of transaction inited: %d\n", noTxs)
	},
}

func getMsigContractFromParams(args []string) (msigAddress string, err error) {
	if len(args) < 1 {
		fmt.Printf("Jarvis: Please specify multisig address\n")
		return "", fmt.Errorf("not enough params")
	}

	addrDesc, err := db.GetAddress(args[0])
	var msigName string
	if err != nil {
		msigName = "Unknown"
		addresses := util.ScanForAddresses(args[0])
		if len(addresses) == 0 {
			fmt.Printf("Couldn't find any address for \"%s\"", args[0])
			return "", fmt.Errorf("address not found")
		}
		msigAddress = addresses[0]
	} else {
		msigName = addrDesc.Desc
		msigAddress = addrDesc.Address
	}
	a, err := util.GetABI(msigAddress, config.Network)
	if err != nil {
		fmt.Printf("Couldn't get ABI of %s from etherscan\n", msigAddress)
		return "", err
	}
	isGnosisMultisig, err := util.IsGnosisMultisig(a)
	if err != nil {
		fmt.Printf("Checking failed, %s (%s) is not a contract\n", msigAddress, msigName)
		return "", err
	}
	if !isGnosisMultisig {
		fmt.Printf("Jarvis: %s (%s) is not a Gnosis multisig or not with a version I understand.\n", msigAddress, msigName)
		return "", fmt.Errorf("not gnosis multisig")
	}
	fmt.Printf("Multisig: %s (%s)\n", msigAddress, msigName)
	return msigAddress, nil
}

func handleApproveOrRevokeOrExecuteMsig(method string, cmd *cobra.Command, args []string) {
	reader, err := util.EthReader(config.Network)
	if err != nil {
		fmt.Printf("Couldn't connect to blockchain.\n")
		return
	}

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

	showMsigTxInfo(multisigContract, txid)
	// TODO: support multiple txs?

	var a *abi.ABI
	if config.ForceERC20ABI {
		a, err = ethutils.GetERC20ABI()
	} else if config.CustomABI != "" {
		a, err = util.ReadCustomABI(config.CustomABI, config.Network)
	} else {
		a, err = util.GetABI(config.To, config.Network)
	}
	if err != nil {
		fmt.Printf("Couldn't get the ABI: %s\n", err)
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

	err = promptTxConfirmation(config.From, tx)
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
	util.DisplayWaitAnalyze(
		tx, broadcasted, err, config.Network,
	)
}

var revokeMsigCmd = &cobra.Command{
	Use:               "revoke",
	Short:             "Revoke gnosis transaction",
	Long:              ``,
	PersistentPreRunE: CommonTxPreprocess,
	Run: func(cmd *cobra.Command, args []string) {
		handleApproveOrRevokeOrExecuteMsig("revokeConfirmation", cmd, args)
	},
}

var executeMsigCmd = &cobra.Command{
	Use:               "execute",
	Short:             "Execute a confirmed gnosis transaction",
	Long:              ``,
	PersistentPreRunE: CommonTxPreprocess,
	Run: func(cmd *cobra.Command, args []string) {
		handleApproveOrRevokeOrExecuteMsig("executeTransaction", cmd, args)
	},
}

var approveMsigCmd = &cobra.Command{
	Use:               "approve",
	Short:             "Approve gnosis transaction",
	Long:              ``,
	PersistentPreRunE: CommonTxPreprocess,
	Run: func(cmd *cobra.Command, args []string) {
		handleApproveOrRevokeOrExecuteMsig("confirmTransaction", cmd, args)
	},
}

var initMsigCmd = &cobra.Command{
	Use:   "init",
	Short: "Init gnosis transaction",
	Long:  ``,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		CommonTxPreprocess(cmd, args)

		if config.MsigValue < 0 {
			return fmt.Errorf("multisig value can't be negative")
		}

		var msigToName string
		config.MsigTo, msigToName, err = util.GetAddressFromString(config.MsigTo)
		if err != nil {
			return err
		}
		fmt.Printf("Call to: %s (%s)\n", config.MsigTo, msigToName)
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		reader, err := util.EthReader(config.Network)
		if err != nil {
			fmt.Printf("Couldn't connect to blockchain.\n")
			return
		}

		data, err := promptTxData(config.MsigTo, config.PrefillParams, config.ForceERC20ABI, config.CustomABI)
		if err != nil {
			fmt.Printf("Couldn't pack multisig calling data: %s\n", err)
			fmt.Printf("Continue with EMPTY CALLING DATA\n")
			data = []byte{}
		}

		a, err := util.GetABI(config.To, config.Network)
		if err != nil {
			fmt.Printf("Couldn't get the multisig's ABI: %s\n", err)
			return
		}

		txdata, err := a.Pack(
			"submitTransaction",
			ethutils.HexToAddress(config.MsigTo),
			ethutils.FloatToBigInt(config.MsigValue, 18),
			data,
		)
		if err != nil {
			fmt.Printf("Couldn't pack tx data: %s\n", err)
			return
		}

		// var GasLimit uint64
		if config.GasLimit == 0 {
			config.GasLimit, err = reader.EstimateGas(config.From, config.To, config.GasPrice+config.ExtraGasPrice, config.Value, txdata)
			if err != nil {
				fmt.Printf("Couldn't estimate gas limit: %s\n", err)
				return
			}
		}

		tx := ethutils.BuildTx(config.Nonce, config.To, config.Value, config.GasLimit+config.ExtraGasLimit, config.GasPrice+config.ExtraGasPrice, txdata)

		err = promptTxConfirmation(config.From, tx)
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
		util.DisplayWaitAnalyze(
			tx, broadcasted, err, config.Network,
		)
	},
}

var msigCmd = &cobra.Command{
	Use:   "msig",
	Short: "Gnosis multisig operations",
	Long:  ``,
}

func init() {
	msigCmd.AddCommand(summaryMsigCmd)
	msigCmd.AddCommand(transactionInfoMsigCmd)
	msigCmd.AddCommand(govInfoMsigCmd)

	initMsigCmd.Flags().Float64VarP(&config.MsigValue, "msig-value", "l", 0, "Amount of eth to send with the multisig. It is in ETH, not WEI.")
	initMsigCmd.Flags().StringVarP(&config.MsigTo, "msig-to", "j", "", "Target address the multisig will interact with. Can be address or name.")
	initMsigCmd.Flags().Uint64VarP(&config.MethodIndex, "method-index", "M", 0, "Index of the method in alphabeth sorted method list of the contract. Index counts from 1.")
	initMsigCmd.Flags().StringVarP(&config.PrefillStr, "prefills", "I", "", "Prefill params string. Each param is separated by | char. If the param is \"?\", user input will be prompted.")
	initMsigCmd.MarkFlagRequired("msig-to")

	writeCmds := []*cobra.Command{
		approveMsigCmd,
		revokeMsigCmd,
		initMsigCmd,
		executeMsigCmd,
	}
	for _, c := range writeCmds {
		c.PersistentFlags().Float64VarP(&config.GasPrice, "gasprice", "p", 0, "Gas price in gwei. If default value is used, we will use https://ethgasstation.info/ to get fast gas price. The gas price to be used in the tx is gas price + extra gas price")
		c.PersistentFlags().Float64VarP(&config.ExtraGasPrice, "extraprice", "P", 0, "Extra gas price in gwei. The gas price to be used in the tx is gas price + extra gas price")
		c.PersistentFlags().Uint64VarP(&config.GasLimit, "gas", "g", 0, "Base gas limit for the tx. If default value is used, we will use ethereum nodes to estimate the gas limit. The gas limit to be used in the tx is gas limit + extra gas limit")
		c.PersistentFlags().Uint64VarP(&config.ExtraGasLimit, "extragas", "G", 250000, "Extra gas limit for the tx. The gas limit to be used in the tx is gas limit + extra gas limit")
		c.PersistentFlags().Uint64VarP(&config.Nonce, "nonce", "n", 0, "Nonce of the from account. If default value is used, we will use the next available nonce of from account")
		c.PersistentFlags().StringVarP(&config.From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
		c.Flags().Float64VarP(&config.Value, "amount", "v", 0, "Amount of eth to send. It is in eth value, not wei.")
		c.PersistentFlags().BoolVarP(&config.ForceERC20ABI, "erc20-abi", "e", false, "Use ERC20 ABI where possible.")
		c.PersistentFlags().StringVarP(&config.CustomABI, "abi", "c", "", "Custom abi. It can be either an address, a path to an abi file or an url to an abi. If it is an address, the abi of that address from etherscan will be queried. This param only takes effect if erc20-abi param is not true.")
	}

	msigCmd.AddCommand(approveMsigCmd)
	msigCmd.AddCommand(revokeMsigCmd)
	msigCmd.AddCommand(initMsigCmd)
	msigCmd.AddCommand(executeMsigCmd)
	rootCmd.AddCommand(msigCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// txCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// txCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
