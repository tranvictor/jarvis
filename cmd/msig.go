package cmd

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/cobra"
	"github.com/tranvictor/walletarmy"

	"github.com/tranvictor/jarvis/accounts"
	jtypes "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
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
			config.Network(),
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
			config.Network(),
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

		cmdutil.AnalyzeAndShowMsigTxInfo(multisigContract, txid, config.Network())
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
			config.Network(),
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
	a, err := util.GetABI(msigAddress, config.Network())
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

func GetApproverAccountFromMsig(multisigContract *msig.MultisigContract) (string, error) {
	owners, err := multisigContract.Owners()
	if err != nil {
		return "", fmt.Errorf("getting msig owners failed: %w", err)
	}

	var acc jtypes.AccDesc
	count := 0
	for _, owner := range owners {
		a, err := accounts.GetAccount(owner)
		if err == nil {
			acc = a
			count++
		}
	}
	if count == 0 {
		return "", fmt.Errorf("you don't have any wallet which is this multisig signer. please jarvis wallet add to add the wallet")
	}
	if count != 1 {
		fmt.Printf("you have many wallets that are this multisig signers. select the last one")
	}
	return acc.Address, nil
}

var batchApproveMsigCmd = &cobra.Command{
	Use:   "bapprove",
	Short: "Approve multiple gnosis transactions",
	Long:  `This command only works with list of init transactions as the first param`,
	Run: func(cmd *cobra.Command, args []string) {
		cm := walletarmy.NewWalletManager()
		a := util.GetGnosisMsigABI()
		// get networks and txs from params
		networks, txs := cmdutil.ScanForTxs(args[0])
		if len(networks) == 0 || len(txs) == 0 {
			fmt.Printf("No txs passed to the first param. Did nothing.\n")
			return
		}

		for i, n := range networks {
			txHash := txs[i]
			network, err := jarvisnetworks.GetNetwork(n)
			if err != nil {
				fmt.Printf("%s network is not supported. Skip this tx.\n", n)
				continue
			}
			// getting msig info
			txinfo, err := cm.Reader(network).TxInfoFromHash(txHash)
			if err != nil {
				fmt.Printf("Couldn't get tx info from hash: %s. Skip this tx.\n", err)
				continue
			}
			if txinfo.Receipt == nil {
				fmt.Printf("This tx is still pending. Skip this tx.\n")
				continue
			}
			var txid *big.Int
			msigHex := txinfo.Tx.To().Hex()
			for _, l := range txinfo.Receipt.Logs {
				if strings.EqualFold(l.Address.Hex(), msigHex) &&
					strings.EqualFold(l.Topics[0].Hex(), "0xc0ba8fe4b176c1714197d43b9cc6bcf797a4a7461c5fe8d0ef6e184ae7601e51") {

					txid = l.Topics[1].Big()
					break
				}
			}
			if txid == nil {
				fmt.Printf("This tx is not a gnosis classic multisig init tx. Skip this tx.\n")
				continue
			}

			multisigContract, err := msig.NewMultisigContract(
				msigHex,
				network,
			)
			if err != nil {
				fmt.Printf("Couldn't interact with the contract: %s. Skip this tx.\n", err)
				continue
			}

			from, err := GetApproverAccountFromMsig(multisigContract)
			if err != nil {
				fmt.Printf("Couldn't read and get wallet to approve this msig. You might not have any approver wallets.\n")
				continue
			}

			_, confirmed, executed := cmdutil.AnalyzeAndShowMsigTxInfo(multisigContract, txid, network)
			if executed {
				fmt.Printf("This tx is already executed. You don't have to approve it anymore. Continue with next tx.\n")
				continue
			}

			if confirmed {
				fmt.Printf("This tx is already confirmed but not yet executed. You should execute it instead of approving it. Continue to next tx.\n")
				continue
			}

			data, err := a.Pack("confirmTransaction", txid)
			if err != nil {
				fmt.Printf("Couldn't pack data: %s. Continue with next tx.\n", err)
				continue
			}

			txType, err := cmdutil.ValidTxType(cm.Reader(network), network)
			if err != nil {
				fmt.Printf("Couldn't determine proper tx type: %s\n", err)
				return
			}

			if txType == types.LegacyTxType && config.TxType == types.DynamicFeeTxType {
				jarviscommon.DebugPrintf("The %s network doesn't support dynamic fee transaction, ignore tx type in cmd parameters", network.GetName())
			}

			if txType == types.DynamicFeeTxType && config.TxType == types.LegacyTxType {
				txType = config.TxType
			}

			if config.TxType == types.LegacyTxType && config.TipGas > 0 {
				fmt.Printf("We are doing legacy tx hence we ignore tip gas parameter.\n")
			}

			minedTx, err := cm.EnsureTxWithHooks(
				txType,
				jarviscommon.HexToAddress(from), jarviscommon.HexToAddress(msigHex), // from, to
				nil, // value
				0,   // gasLimit
				0,   // gasPrice
				0,   // tipCap
				data,
				network,
				func(tx *types.Transaction, buildError error) error { // before signing and broadcasting hook
					if buildError != nil {
						fmt.Printf("Couldn't build tx: %s\n", buildError)
						return buildError
					}

					// prompt user to confirm the tx
					err = cmdutil.PromptTxConfirmation(
						cm.Analyzer(network),
						util.GetJarvisAddress(from, network),
						tx,
						nil,
						network,
					)
					if err != nil {
						fmt.Printf("Skip this tx. Continue with next tx.\n")
						return fmt.Errorf("user aborted: %w", err)
					}
					return nil
				},
				func(broadcastedTx *types.Transaction, signError error) error { // after signing hook
					if signError != nil {
						return signError
					}

					if config.DontWaitToBeMined {
						util.DisplayBroadcastedTx(
							broadcastedTx, true, signError, network,
						)
						return fmt.Errorf("not waiting for tx to be mined")
					}
					return nil
				},
			)

			util.DisplayWaitAnalyze(
				cm.Reader(network), cm.Analyzer(network), minedTx, true, nil, network,
				a, nil, config.DegenMode,
			)
		}
	},
}

var newMsigCmd = &cobra.Command{
	Use:               "new",
	Short:             "deploy a new gnosis classic multisig",
	Long:              ` `,
	TraverseChildren:  true,
	PersistentPreRunE: cmdutil.CommonTxPreprocess,
	Run: func(cmd *cobra.Command, args []string) {
		reader, err := util.EthReader(config.Network())
		if err != nil {
			fmt.Printf("Couldn't connect to blockchain.\n")
			return
		}

		analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

		msigABI := util.GetGnosisMsigABI()

		cAddr := crypto.CreateAddress(jarviscommon.HexToAddress(config.From), config.Nonce).Hex()

		data, err := cmdutil.PromptTxData(
			analyzer,
			cAddr,
			cmdutil.CONSTRUCTOR_METHOD_INDEX,
			config.PrefillParams,
			config.PrefillMode,
			msigABI,
			nil,
			config.Network(),
		)
		if err != nil {
			fmt.Printf("Couldn't pack constructor data: %s\n", err)
			return
		}

		bytecode, err := util.GetGnosisMsigDeployByteCode(data)
		if err != nil {
			fmt.Printf("Couldn't pack deployment data: %s\n", err)
			return
		}

		customABIs := map[string]*abi.ABI{
			strings.ToLower(cAddr): msigABI,
		}

		// var GasLimit uint64
		if config.GasLimit == 0 {
			config.GasLimit, err = reader.EstimateExactGas(config.From, "", 0, config.Value, bytecode)
			if err != nil {
				fmt.Printf("Couldn't estimate gas limit: %s\n", err)
				return
			}
		}
		tx := jarviscommon.BuildContractCreationTx(
			config.TxType,
			config.Nonce,
			config.Value,
			config.GasLimit+config.ExtraGasLimit,
			config.GasPrice+config.ExtraGasPrice,
			config.TipGas+config.ExtraTipGas,
			bytecode,
			config.Network().GetChainID(),
		)

		err = cmdutil.PromptTxConfirmation(
			analyzer,
			util.GetJarvisAddress(config.From, config.Network()),
			tx,
			customABIs,
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

		signedAddr, signedTx, err := account.SignTx(tx, big.NewInt(int64(config.Network().GetChainID())))
		if err != nil {
			fmt.Printf("Failed to sign tx: %s\n", err)
			return
		}
		if signedAddr.Cmp(jarviscommon.HexToAddress(config.FromAcc.Address)) != 0 {
			fmt.Printf(
				"Signed from wrong address. You could use wrong hw or passphrase. Expected wallet: %s, signed wallet: %s\n",
				config.FromAcc.Address,
				signedAddr.Hex(),
			)
			return
		}

		broadcasted, err := cmdutil.HandlePostSign(signedTx, reader, analyzer, nil)
		if err != nil && !broadcasted {
			fmt.Printf("Failed to proceed after signing the tx: %s. Aborted.\n", err)
		}
	},
}

// var initMsigSend = &cobra.Command{
// 	Use:   "init",
// 	Short: "Init gnosis transaction",
// 	Long:  ``,
// 	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
// 	},
// }

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
		reader, err := util.EthReader(config.Network())
		if err != nil {
			fmt.Printf("Couldn't connect to blockchain.\n")
			return
		}

		analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

		a, err := util.ConfigToABI(config.MsigTo, config.ForceERC20ABI, config.CustomABI, config.Network())
		if err != nil {
			fmt.Printf("Couldn't get abi for %s: %s. Continue:\n", config.MsigTo, err)
		}

		data := []byte{}
		if a != nil && !config.NoFuncCall {
			data, err = cmdutil.PromptTxData(
				analyzer,
				config.MsigTo,
				config.MethodIndex,
				config.PrefillParams,
				config.PrefillMode,
				a,
				nil,
				config.Network(),
			)
			if err != nil {
				fmt.Printf("Couldn't pack multisig calling data: %s\n", err)
				fmt.Printf("Continue with EMPTY CALLING DATA\n")
				data = []byte{}
			}
		}

		msigABI, err := util.GetABI(config.To, config.Network())
		if err != nil {
			fmt.Printf("Couldn't get the multisig's ABI: %s\n", err)
			return
		}

		if config.Simulate {
			multisigContract, err := msig.NewMultisigContract(
				config.To,
				config.Network(),
			)
			if err != nil {
				fmt.Printf("Couldn't interact with the contract: %s\n", err)
				return
			}
			err = multisigContract.SimulateSubmit(config.From, config.MsigTo, jarviscommon.FloatToBigInt(config.MsigValue, config.Network().GetNativeTokenDecimal()), data)
			if err != nil {
				fmt.Printf("Could not simulate interact with the contract: %s\n", err)
				return
			}
		}

		txdata, err := msigABI.Pack(
			"submitTransaction",
			jarviscommon.HexToAddress(config.MsigTo),
			jarviscommon.FloatToBigInt(config.MsigValue, config.Network().GetNativeTokenDecimal()),
			data,
		)
		if err != nil {
			fmt.Printf("Couldn't pack tx data: %s\n", err)
			return
		}

		// var GasLimit uint64
		if config.GasLimit == 0 {
			config.GasLimit, err = reader.EstimateExactGas(config.From, config.To, 0, config.Value, txdata)
			if err != nil {
				fmt.Printf("Couldn't estimate gas limit: %s\n", err)
				return
			}
		}

		tx := jarviscommon.BuildExactTx(
			config.TxType,
			config.Nonce,
			config.To,
			config.Value,
			config.GasLimit+config.ExtraGasLimit,
			config.GasPrice+config.ExtraGasPrice,
			config.TipGas+config.ExtraTipGas,
			txdata,
			config.Network().GetChainID(),
		)

		customABIs := map[string]*abi.ABI{
			strings.ToLower(config.MsigTo): a,
			strings.ToLower(config.To):     msigABI,
		}
		err = cmdutil.PromptTxConfirmation(
			analyzer,
			util.GetJarvisAddress(config.From, config.Network()),
			tx,
			customABIs,
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

		signedAddr, signedTx, err := account.SignTx(tx, big.NewInt(int64(config.Network().GetChainID())))
		if err != nil {
			fmt.Printf("Failed to sign tx: %s\n", err)
			return
		}
		if signedAddr.Cmp(jarviscommon.HexToAddress(config.FromAcc.Address)) != 0 {
			fmt.Printf("Signed from wrong address. You could use wrong hw or passphrase. Expected wallet: %s, signed wallet: %s\n",
				config.FromAcc.Address,
				signedAddr.Hex(),
			)
			return
		}

		broadcasted, err := cmdutil.HandlePostSign(signedTx, reader, analyzer, a)
		if err != nil && !broadcasted {
			fmt.Printf("Failed to proceed after signing the tx: %s. Aborted.\n", err)
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
	// msigCmd.AddCommand(initMsigSend)

	initMsigCmd.Flags().Float64VarP(&config.MsigValue, "msig-value", "V", 0, "Amount of native tokens (eth, bnb, matic...) to send with the multisig. It is in native tokens, not WEI.")
	initMsigCmd.Flags().StringVarP(&config.MsigTo, "msig-to", "j", "", "Target address the multisig will interact with. Can be address or name.")
	initMsigCmd.Flags().Uint64VarP(&config.MethodIndex, "method-index", "M", 0, "Index of the method in alphabeth sorted method list of the contract. Index counts from 1.")
	initMsigCmd.Flags().BoolVarP(&config.NoFuncCall, "no-func-call", "N", false, "True: will not send any data to multisig destination.")
	initMsigCmd.Flags().StringVarP(&config.PrefillStr, "prefills", "I", "", "Prefill params string. Each param is separated by | char. If the param is \"?\", user input will be prompted.")
	initMsigCmd.Flags().BoolVarP(&config.Simulate, "simulate", "S", false, "True: Simulate execution of underlying call.")
	initMsigCmd.MarkFlagRequired("msig-to")

	writeCmds := []*cobra.Command{
		approveMsigCmd,
		revokeMsigCmd,
		initMsigCmd,
		executeMsigCmd,
	}
	for _, c := range writeCmds {
		AddCommonFlagsToTransactionalCmds(c)
		c.Flags().StringVarP(&config.RawValue, "amount", "v", "0", "Amount of eth to send. It is in native token value, not wei.")
		c.PersistentFlags().BoolVarP(&config.ForceERC20ABI, "erc20-abi", "e", false, "Use ERC20 ABI where possible.")
		c.PersistentFlags().StringVarP(&config.CustomABI, "abi", "c", "", "Custom abi. It can be either an address, a path to an abi file or an url to an abi. If it is an address, the abi of that address from etherscan will be queried. This param only takes effect if erc20-abi param is not true.")
	}

	AddCommonFlagsToTransactionalCmds(newMsigCmd)
	newMsigCmd.PersistentFlags().StringVarP(&config.PrefillStr, "prefills", "I", "", "Prefill params string. Each param is separated by | char. If the param is \"?\", user input will be prompted.")
	newMsigCmd.MarkFlagRequired("from")

	batchApproveMsigCmd.PersistentFlags().StringVarP(&config.PrefillStr, "prefills", "I", "", "Prefill params string. Each param is separated by | char. If the param is \"?\", user input will be prompted.")
	batchApproveMsigCmd.PersistentFlags().BoolVarP(&config.DontWaitToBeMined, "no-wait", "F", false, "Will not wait the tx to be mined.")
	batchApproveMsigCmd.PersistentFlags().BoolVarP(&config.YesToAllPrompt, "auto-yes", "y", false, "Don't prompt Yes/No before signing.")
	batchApproveMsigCmd.PersistentFlags().BoolVarP(&config.ForceLegacy, "legacy-tx", "L", false, "Force using legacy transaction")

	msigCmd.AddCommand(approveMsigCmd)
	msigCmd.AddCommand(batchApproveMsigCmd)
	msigCmd.AddCommand(revokeMsigCmd)
	msigCmd.AddCommand(initMsigCmd)
	msigCmd.AddCommand(executeMsigCmd)
	msigCmd.AddCommand(newMsigCmd)
	rootCmd.AddCommand(msigCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// txCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// txCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
