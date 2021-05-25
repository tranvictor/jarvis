package cmd

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/accounts"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/txanalyzer"
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

		cmdutil.AnalyzeAndShowMsigTxInfo(multisigContract, txid)
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
			_, name, err := util.GetMatchingAddress(owner)
			if err != nil {
				fmt.Printf("%d. %s (Unknown)\n", i+1, owner)
			} else {
				fmt.Printf("%d. %s (%s)\n", i+1, owner, name)
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

	addr, name, err := util.GetMatchingAddress(args[0])
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
		msigName = name
		msigAddress = addr
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

var revokeMsigCmd = &cobra.Command{
	Use:               "revoke",
	Short:             "Revoke gnosis transaction",
	Long:              ``,
	PersistentPreRunE: cmdutil.CommonTxPreprocess,
	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.HandleApproveOrRevokeOrExecuteMsig("revokeConfirmation", cmd, args, nil)
	},
}

var executeMsigCmd = &cobra.Command{
	Use:               "execute",
	Short:             "Execute a confirmed gnosis transaction",
	Long:              ``,
	PersistentPreRunE: cmdutil.CommonTxPreprocess,
	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.HandleApproveOrRevokeOrExecuteMsig("executeTransaction", cmd, args, nil)
	},
}

var approveMsigCmd = &cobra.Command{
	Use:               "approve",
	Short:             "Approve gnosis transaction",
	Long:              ``,
	PersistentPreRunE: cmdutil.CommonTxPreprocess,
	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.HandleApproveOrRevokeOrExecuteMsig("confirmTransaction", cmd, args, nil)
	},
}

var initMsigCmd = &cobra.Command{
	Use:   "init",
	Short: "Init gnosis transaction",
	Long:  ``,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		cmdutil.CommonTxPreprocess(cmd, args)

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

		analyzer := txanalyzer.NewGenericAnalyzer(reader)

		a, err := util.ConfigToABI(config.MsigTo, config.ForceERC20ABI, config.CustomABI, config.Network)
		if err != nil {
			fmt.Printf("Couldn't get abi for %s: %s. Continue:\n", config.MsigTo, err)
		}

		data := []byte{}
		if a != nil && !config.NoFuncCall {
			data, err = util.PromptTxData(
				analyzer,
				config.MsigTo,
				config.MethodIndex,
				config.PrefillParams,
				config.PrefillMode,
				a,
				nil,
				config.Network,
			)
			if err != nil {
				fmt.Printf("Couldn't pack multisig calling data: %s\n", err)
				fmt.Printf("Continue with EMPTY CALLING DATA\n")
				data = []byte{}
			}
		}

		msigABI, err := util.GetABI(config.To, config.Network)
		if err != nil {
			fmt.Printf("Couldn't get the multisig's ABI: %s\n", err)
			return
		}

		txdata, err := msigABI.Pack(
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
			config.GasLimit, err = reader.EstimateExactGas(config.From, config.To, config.GasPrice+config.ExtraGasPrice, config.Value, txdata)
			if err != nil {
				fmt.Printf("Couldn't estimate gas limit: %s\n", err)
				return
			}
		}

		tx := ethutils.BuildExactTx(config.Nonce, config.To, config.Value, config.GasLimit+config.ExtraGasLimit, config.GasPrice+config.ExtraGasPrice, txdata)

		customABIs := map[string]*abi.ABI{
			strings.ToLower(config.MsigTo): a,
			strings.ToLower(config.To):     msigABI,
		}
		err = util.PromptTxConfirmation(
			analyzer,
			util.GetJarvisAddress(config.From, config.Network),
			tx,
			customABIs,
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

		if config.DontBroadcast {
			signedTx, err := account.SignTx(tx)
			if err != nil {
				fmt.Printf("%s", err)
				return
			}
			data, err := rlp.EncodeToBytes(signedTx)
			if err != nil {
				fmt.Printf("Couldn't encode the signed tx: %s", err)
				return
			}
			fmt.Printf("Signed tx: %s\n", hexutil.Encode(data))
		} else {
			tx, broadcasted, err := account.SignTxAndBroadcast(tx)
			if config.DontWaitToBeMined {
				util.DisplayBroadcastedTx(
					tx, broadcasted, err, config.Network,
				)
			} else {
				util.DisplayWaitAnalyze(
					reader, analyzer, tx, broadcasted, err, config.Network,
					msigABI, customABIs,
				)
			}
		}
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

	initMsigCmd.Flags().Float64VarP(&config.MsigValue, "msig-value", "V", 0, "Amount of eth to send with the multisig. It is in ETH, not WEI.")
	initMsigCmd.Flags().StringVarP(&config.MsigTo, "msig-to", "j", "", "Target address the multisig will interact with. Can be address or name.")
	initMsigCmd.Flags().Uint64VarP(&config.MethodIndex, "method-index", "M", 0, "Index of the method in alphabeth sorted method list of the contract. Index counts from 1.")
	initMsigCmd.Flags().BoolVarP(&config.NoFuncCall, "no-func-call", "N", false, "True: will not send any data to multisig destination.")
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
		c.Flags().StringVarP(&config.RawValue, "amount", "v", "0", "Amount of eth to send. It is in eth value, not wei.")
		c.PersistentFlags().BoolVarP(&config.ForceERC20ABI, "erc20-abi", "e", false, "Use ERC20 ABI where possible.")
		c.PersistentFlags().StringVarP(&config.CustomABI, "abi", "c", "", "Custom abi. It can be either an address, a path to an abi file or an url to an abi. If it is an address, the abi of that address from etherscan will be queried. This param only takes effect if erc20-abi param is not true.")
		c.PersistentFlags().BoolVarP(&config.DontWaitToBeMined, "no-wait", "F", false, "Will not wait the tx to be mined.")
		c.PersistentFlags().BoolVarP(&config.DontBroadcast, "dry", "d", false, "Will not broadcast the tx, only show signed tx.")
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
