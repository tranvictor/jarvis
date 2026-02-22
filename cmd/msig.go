package cmd

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

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

var ErrUserAborted = errors.New("user aborted")
var ErrNotWaitingForMining = errors.New("not waiting for mining")

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
			appUI.Error("Couldn't interact with the contract: %s", err)
			return
		}

		noTxs, err := multisigContract.NOTransactions()
		if err != nil {
			appUI.Error("Couldn't get number of transactions of the multisig: %s", err)
			return
		}
		appUI.Info("Number of transactions: %d", noTxs)
		noExecuted := 0
		noRevoked := 0
		noConfirmed := 0
		noError := 0
		nonExecutedConfirmedIds := []int64{}
		for i := int64(0); i < noTxs; i++ {
			executed, err := multisigContract.IsExecuted(big.NewInt(i))
			if err != nil {
				appUI.Error("%d. %s", i, err)
				noError++
				continue
			}
			if executed {
				appUI.Info("%d. executed", i)
				noExecuted++
				noConfirmed++
				continue
			}
			confirmed, err := multisigContract.IsConfirmed(big.NewInt(i))
			if err != nil {
				appUI.Error("%d. %s", i, err)
				noError++
				continue
			}
			if confirmed {
				appUI.Info("%d. confirmed - not executed", i)
				nonExecutedConfirmedIds = append(nonExecutedConfirmedIds, i)
				noConfirmed++
				continue
			}
			appUI.Info("%d. unconfirmed", i)
		}
		appUI.Info("------------")
		appUI.Info("Total executed txs: %d", noExecuted)
		appUI.Info("Total confirmed but NOT executed txs: %d. Their ids are: %v", noConfirmed-noExecuted, nonExecutedConfirmedIds)
		appUI.Info("Total revoked txs: %d", noRevoked)
		appUI.Info("Total unconfirmed txs (including unknown txs): %d", int(noTxs)-noConfirmed-noRevoked)
		_ = noError
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
			appUI.Error("Couldn't interact with the contract: %s", err)
			return
		}

		if len(args) < 2 {
			appUI.Error("Please specify tx id in either hex or int format after the multisig address")
			return
		}

		idStr := strings.Trim(args[1], " ")
		txid, err := util.ParamToBigInt(idStr)
		if err != nil {
			appUI.Error("Can't convert \"%s\" to int.", idStr)
			return
		}

		cmdutil.AnalyzeAndShowMsigTxInfo(appUI, multisigContract, txid, config.Network())
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
			appUI.Error("Couldn't interact with the contract: %s", err)
			return
		}

		appUI.Info("Owner list:")
		owners, err := multisigContract.Owners()
		if err != nil {
			appUI.Error("Couldn't get owners of the multisig: %s", err)
			return
		}
		for i, owner := range owners {
			_, name, err := util.GetMatchingAddress(owner)
			if err != nil {
				appUI.Info("%d. %s (Unknown)", i+1, owner)
			} else {
				appUI.Info("%d. %s (%s)", i+1, owner, name)
			}
		}
		voteRequirement, err := multisigContract.VoteRequirement()
		if err != nil {
			appUI.Error("Couldn't get vote requirements of the multisig: %s", err)
			return
		}
		appUI.Info("Vote requirement: %d/%d", voteRequirement, len(owners))
		noTxs, err := multisigContract.NOTransactions()
		if err != nil {
			appUI.Error("Couldn't get number of transactions of the multisig: %s", err)
			return
		}
		appUI.Info("Number of transaction inited: %d", noTxs)
	},
}

func getMsigContractFromParams(args []string) (msigAddress string, err error) {
	if len(args) < 1 {
		appUI.Error("Please specify multisig address")
		return "", fmt.Errorf("not enough params")
	}

	addr, name, err := util.GetMatchingAddress(args[0])
	var msigName string
	if err != nil {
		msigName = "Unknown"
		addresses := util.ScanForAddresses(args[0])
		if len(addresses) == 0 {
			appUI.Error("Couldn't find any address for \"%s\"", args[0])
			return "", fmt.Errorf("address not found")
		}
		msigAddress = addresses[0]
	} else {
		msigName = name
		msigAddress = addr
	}
	a, err := util.GetABI(msigAddress, config.Network())
	if err != nil {
		appUI.Error("Couldn't get ABI of %s from etherscan", msigAddress)
		return "", err
	}
	isGnosisMultisig, err := util.IsGnosisMultisig(a)
	if err != nil {
		appUI.Error("Checking failed, %s (%s) is not a contract", msigAddress, msigName)
		return "", err
	}
	if !isGnosisMultisig {
		appUI.Error("%s (%s) is not a Gnosis multisig or not with a version I understand.", msigAddress, msigName)
		return "", fmt.Errorf("not gnosis multisig")
	}
	appUI.Info("Multisig: %s (%s)", msigAddress, msigName)
	return msigAddress, nil
}

var revokeMsigCmd = &cobra.Command{
	Use:   "revoke",
	Short: "Revoke gnosis transaction",
	Long:  ``,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonTxPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.HandleApproveOrRevokeOrExecuteMsig(appUI, "revokeConfirmation", cmd, args, nil)
	},
}

var executeMsigCmd = &cobra.Command{
	Use:   "execute",
	Short: "Execute a confirmed gnosis transaction",
	Long:  ``,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonTxPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.HandleApproveOrRevokeOrExecuteMsig(appUI, "executeTransaction", cmd, args, nil)
	},
}

var approveMsigCmd = &cobra.Command{
	Use:   "approve",
	Short: "Approve gnosis transaction",
	Long:  ``,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonTxPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.HandleApproveOrRevokeOrExecuteMsig(appUI, "confirmTransaction", cmd, args, nil)
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
		appUI.Warn("you have many wallets that are this multisig signers. selecting the last one")
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
		networks, txs := cmdutil.ScanForTxs(args[0])
		if len(networks) == 0 || len(txs) == 0 {
			appUI.Error("No txs passed to the first param. Did nothing.")
			return
		}

		for i, n := range networks {
			txHash := txs[i]
			network, err := jarvisnetworks.GetNetwork(n)
			if err != nil {
				appUI.Error("%s network is not supported. Skip this tx.", n)
				continue
			}
			txinfo, err := cm.Reader(network).TxInfoFromHash(txHash)
			if err != nil {
				appUI.Error("Couldn't get tx info from hash: %s. Skip this tx.", err)
				continue
			}
			if txinfo.Receipt == nil {
				appUI.Warn("This tx is still pending. Skip this tx.")
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
				appUI.Warn("This tx is not a gnosis classic multisig init tx. Skip this tx.")
				continue
			}

			multisigContract, err := msig.NewMultisigContract(msigHex, network)
			if err != nil {
				appUI.Error("Couldn't interact with the contract: %s. Skip this tx.", err)
				continue
			}

			from, err := GetApproverAccountFromMsig(multisigContract)
			if err != nil {
				appUI.Error("Couldn't read and get wallet to approve this msig. You might not have any approver wallets.")
				continue
			}

			_, confirmed, executed := cmdutil.AnalyzeAndShowMsigTxInfo(appUI, multisigContract, txid, network)
			if executed {
				appUI.Warn("This tx is already executed. You don't have to approve it anymore. Continue with next tx.")
				continue
			}
			if confirmed {
				appUI.Warn("This tx is already confirmed but not yet executed. You should execute it instead of approving it. Continue to next tx.")
				continue
			}

			data, err := a.Pack("confirmTransaction", txid)
			if err != nil {
				appUI.Error("Couldn't pack data: %s. Continue with next tx.", err)
				continue
			}

			txType, err := cmdutil.ValidTxType(cm.Reader(network), network)
			if err != nil {
				appUI.Error("Couldn't determine proper tx type: %s", err)
				return
			}

			if txType == types.LegacyTxType && config.TxType == types.DynamicFeeTxType {
				jarviscommon.DebugPrintf("The %s network doesn't support dynamic fee transaction, ignore tx type in cmd parameters", network.GetName())
			}

			if txType == types.DynamicFeeTxType && config.TxType == types.LegacyTxType {
				txType = config.TxType
			}

			if config.TxType == types.LegacyTxType && config.TipGas > 0 {
				appUI.Warn("We are doing legacy tx hence we ignore tip gas parameter.")
			}

			minedTx, err := cm.EnsureTxWithHooks(
				10,
				5*time.Second,
				5*time.Second,
				txType,
				jarviscommon.HexToAddress(from), jarviscommon.HexToAddress(msigHex),
				nil,
				0,
				2000000,
				0,
				0,
				0,
				0,
				data,
				network,
				func(tx *types.Transaction, buildError error) error {
					if buildError != nil {
						appUI.Error("Couldn't build tx: %s", buildError)
						return buildError
					}
					err = cmdutil.PromptTxConfirmation(
						appUI,
						cm.Analyzer(network),
						util.GetJarvisAddress(from, network),
						tx,
						nil,
						network,
					)
					if err != nil {
						appUI.Warn("Skip this tx. Continue with next tx.")
						return fmt.Errorf("%w: %w", ErrUserAborted, err)
					}
					return nil
				},
				func(broadcastedTx *types.Transaction, signError error) error {
					if signError != nil {
						return signError
					}
					if broadcastedTx != nil {
						util.DisplayBroadcastedTx(appUI, broadcastedTx, true, signError, network)
					}
					if config.DontWaitToBeMined {
						return fmt.Errorf("%w: %w", ErrNotWaitingForMining, signError)
					}
					return nil
				},
				nil,
				nil,
			)

			if err != nil {
				if !errors.Is(err, ErrUserAborted) && !errors.Is(err, ErrNotWaitingForMining) {
					appUI.Error("Failed to broadcast the tx after retries: %s. Aborted.", err)
				}
				continue
			}

			if !config.DontWaitToBeMined {
				util.AnalyzeAndPrint(
					appUI,
					cm.Reader(network), cm.Analyzer(network),
					minedTx.Hash().Hex(), network, false, "", a, nil, config.DegenMode,
				)
			}
		}
	},
}

var newMsigCmd = &cobra.Command{
	Use:              "new",
	Short:            "deploy a new gnosis classic multisig",
	Long:             ` `,
	TraverseChildren: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonTxPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		reader, err := util.EthReader(config.Network())
		if err != nil {
			appUI.Error("Couldn't connect to blockchain.")
			return
		}

		analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

		msigABI := util.GetGnosisMsigABI()

		cAddr := crypto.CreateAddress(jarviscommon.HexToAddress(config.From), config.Nonce).Hex()

		data, err := cmdutil.PromptTxData(
			appUI,
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
			appUI.Error("Couldn't pack constructor data: %s", err)
			return
		}

		bytecode, err := util.GetGnosisMsigDeployByteCode(data)
		if err != nil {
			appUI.Error("Couldn't pack deployment data: %s", err)
			return
		}

		customABIs := map[string]*abi.ABI{
			strings.ToLower(cAddr): msigABI,
		}

		if config.GasLimit == 0 {
			config.GasLimit, err = reader.EstimateExactGas(config.From, "", 0, config.Value, bytecode)
			if err != nil {
				appUI.Error("Couldn't estimate gas limit: %s", err)
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
			appUI,
			analyzer,
			util.GetJarvisAddress(config.From, config.Network()),
			tx,
			customABIs,
			config.Network(),
		)
		if err != nil {
			appUI.Error("Aborted!")
			return
		}

		appUI.Info("Unlock your wallet and sign now...")
		account, err := accounts.UnlockAccount(config.FromAcc)
		if err != nil {
			appUI.Error("Failed: %s", err)
			return
		}

		signedAddr, signedTx, err := account.SignTx(tx, big.NewInt(int64(config.Network().GetChainID())))
		if err != nil {
			appUI.Error("Failed to sign tx: %s", err)
			return
		}
		if signedAddr.Cmp(jarviscommon.HexToAddress(config.FromAcc.Address)) != 0 {
			appUI.Error(
				"Signed from wrong address. You could use wrong hw or passphrase. Expected wallet: %s, signed wallet: %s",
				config.FromAcc.Address,
				signedAddr.Hex(),
			)
			return
		}

		broadcasted, err := cmdutil.HandlePostSign(appUI, signedTx, reader, analyzer, nil)
		if err != nil && !broadcasted {
			appUI.Error("Failed to proceed after signing the tx: %s. Aborted.", err)
		}
	},
}

var initMsigCmd = &cobra.Command{
	Use:   "init",
	Short: "Init gnosis transaction",
	Long:  ``,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		if err = cmdutil.CommonTxPreprocess(appUI, cmd, args); err != nil {
			return err
		}

		if config.MsigValue < 0 {
			return fmt.Errorf("multisig value can't be negative")
		}

		var msigToName string
		config.MsigTo, msigToName, err = util.GetAddressFromString(config.MsigTo)
		if err != nil {
			return err
		}
		appUI.Info("Call to: %s (%s)", config.MsigTo, msigToName)
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		reader, err := util.EthReader(config.Network())
		if err != nil {
			appUI.Error("Couldn't connect to blockchain.")
			return
		}

		analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

		a, err := util.ConfigToABI(config.MsigTo, config.ForceERC20ABI, config.CustomABI, config.Network())
		if err != nil {
			appUI.Warn("Couldn't get abi for %s: %s. Continue:", config.MsigTo, err)
		}

		data := []byte{}
		if a != nil && !config.NoFuncCall {
			data, err = cmdutil.PromptTxData(
				appUI,
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
				appUI.Error("Couldn't pack multisig calling data: %s", err)
				appUI.Warn("Continue with EMPTY CALLING DATA")
				data = []byte{}
			}
		}

		msigABI, err := util.GetABI(config.To, config.Network())
		if err != nil {
			appUI.Error("Couldn't get the multisig's ABI: %s", err)
			return
		}

		if config.Simulate {
			multisigContract, err := msig.NewMultisigContract(config.To, config.Network())
			if err != nil {
				appUI.Error("Couldn't interact with the contract: %s", err)
				return
			}
			err = multisigContract.SimulateSubmit(config.From, config.MsigTo, jarviscommon.FloatToBigInt(config.MsigValue, config.Network().GetNativeTokenDecimal()), data)
			if err != nil {
				appUI.Error("Could not simulate interact with the contract: %s", err)
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
			appUI.Error("Couldn't pack tx data: %s", err)
			return
		}

		if config.GasLimit == 0 {
			config.GasLimit, err = reader.EstimateExactGas(config.From, config.To, 0, config.Value, txdata)
			if err != nil {
				appUI.Error("Couldn't estimate gas limit: %s", err)
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
			appUI,
			analyzer,
			util.GetJarvisAddress(config.From, config.Network()),
			tx,
			customABIs,
			config.Network(),
		)
		if err != nil {
			appUI.Error("Aborted!")
			return
		}

		appUI.Info("Unlock your wallet and sign now...")
		account, err := accounts.UnlockAccount(config.FromAcc)
		if err != nil {
			appUI.Error("Failed: %s", err)
			return
		}

		signedAddr, signedTx, err := account.SignTx(tx, big.NewInt(int64(config.Network().GetChainID())))
		if err != nil {
			appUI.Error("Failed to sign tx: %s", err)
			return
		}
		if signedAddr.Cmp(jarviscommon.HexToAddress(config.FromAcc.Address)) != 0 {
			appUI.Error(
				"Signed from wrong address. You could use wrong hw or passphrase. Expected wallet: %s, signed wallet: %s",
				config.FromAcc.Address,
				signedAddr.Hex(),
			)
			return
		}

		broadcasted, err := cmdutil.HandlePostSign(appUI, signedTx, reader, analyzer, a)
		if err != nil && !broadcasted {
			appUI.Error("Failed to proceed after signing the tx: %s. Aborted.", err)
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
}
