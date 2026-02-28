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
	"github.com/tranvictor/jarvis/util"
)

var ErrUserAborted = errors.New("user aborted")
var ErrNotWaitingForMining = errors.New("not waiting for mining")

var summaryMsigCmd = &cobra.Command{
	Use:   "summary",
	Short: "Print all txs confirmation and execution status of the multisig",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		msigAddress, err := getMsigContractFromParams(args, cmdutil.DefaultABIResolver{})
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
		noConfirmed := 0
		noError := 0
		nonExecutedConfirmedIds := []int64{}
		for i := int64(0); i < noTxs; i++ {
			executed, err := multisigContract.IsExecuted(big.NewInt(i))
			if err != nil {
				appUI.Error("%d. error: %s", i, err)
				noError++
				continue
			}
			if executed {
				appUI.Success("%d. executed", i)
				noExecuted++
				noConfirmed++
				continue
			}
			confirmed, err := multisigContract.IsConfirmed(big.NewInt(i))
			if err != nil {
				appUI.Error("%d. error: %s", i, err)
				noError++
				continue
			}
			if confirmed {
				appUI.Warn("%d. confirmed - not yet executed", i)
				nonExecutedConfirmedIds = append(nonExecutedConfirmedIds, i)
				noConfirmed++
				continue
			}
			appUI.Info("%d. unconfirmed", i)
		}
		appUI.Info("------------")
		appUI.Info("Total executed txs: %d", noExecuted)
		appUI.Info("Total confirmed but NOT executed txs: %d. IDs: %v", noConfirmed-noExecuted, nonExecutedConfirmedIds)
		appUI.Info("Total unconfirmed txs: %d", int(noTxs)-noConfirmed)
		if noError > 0 {
			appUI.Warn("Txs with query errors (excluded from counts above): %d", noError)
		}
	},
}

var transactionInfoMsigCmd = &cobra.Command{
	Use:   "info",
	Short: "Print all information about a multisig init",
	Long:  ``,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonNetworkPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)

		msigAddress, err := getMsigContractFromParams(args, cmdutil.DefaultABIResolver{})
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

		cmdutil.AnalyzeAndShowMsigTxInfo(appUI, multisigContract, txid, config.Network(), cmdutil.DefaultABIResolver{}, tc.Analyzer)
	},
}

var govInfoMsigCmd = &cobra.Command{
	Use:   "gov",
	Short: "Print goverance information of a Gnosis multisig",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		msigAddress, err := getMsigContractFromParams(args, cmdutil.DefaultABIResolver{})
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

func getMsigContractFromParams(args []string, resolver cmdutil.ABIResolver) (msigAddress string, err error) {
	if len(args) < 1 {
		appUI.Error("Please specify multisig address")
		return "", fmt.Errorf("not enough params")
	}

	addr, name, err := resolver.GetMatchingAddress(args[0])
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
	a, err := resolver.GetABI(msigAddress, config.Network())
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
			if count == 0 {
				acc = a // capture only the first matching wallet
			}
			count++
		}
	}
	if count == 0 {
		return "", fmt.Errorf("you don't have any wallet which is this multisig signer. please jarvis wallet add to add the wallet")
	}
	if count > 1 {
		appUI.Warn("You have %d wallets that are signers of this multisig. Using the first one found: %s", count, acc.Address)
	}
	return acc.Address, nil
}

// batchResult tracks the outcome of each tx in a bapprove batch for the
// summary table rendered at the end.
type batchResult struct {
	Index   int
	Network string
	Summary cmdutil.MsigTxSummary
	Result  string
}

var batchApproveMsigCmd = &cobra.Command{
	Use:   "bapprove",
	Short: "Approve multiple gnosis transactions",
	Long:  `This command only works with list of init transactions as the first param`,
	Run: func(cmd *cobra.Command, args []string) {
		cm := walletarmy.NewWalletManager()
		a := util.GetGnosisMsigABI()
		nwks, txs := cmdutil.ScanForTxs(args[0])
		if len(nwks) == 0 || len(txs) == 0 {
			appUI.Error("No txs passed to the first param. Did nothing.")
			return
		}

		total := len(nwks)
		var results []batchResult

		for i, n := range nwks {
			idx := i + 1
			txHash := txs[i]
			network, err := jarvisnetworks.GetNetwork(n)
			if err != nil {
				appUI.Error("[%d/%d] %s network is not supported. Skip.", idx, total, n)
				results = append(results, batchResult{Index: idx, Network: n, Result: "Error"})
				continue
			}
			txinfo, err := cm.Reader(network).TxInfoFromHash(txHash)
			if err != nil {
				appUI.Error("[%d/%d] Couldn't get tx info: %s. Skip.", idx, total, err)
				results = append(results, batchResult{Index: idx, Network: n, Result: "Error"})
				continue
			}
			if txinfo.Receipt == nil {
				appUI.Warn("[%d/%d] Tx still pending. Skip.", idx, total)
				results = append(results, batchResult{Index: idx, Network: n, Result: "Pending"})
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
				appUI.Warn("[%d/%d] Not a gnosis multisig init tx. Skip.", idx, total)
				results = append(results, batchResult{Index: idx, Network: n, Result: "Skip"})
				continue
			}

			multisigContract, err := msig.NewMultisigContract(msigHex, network)
			if err != nil {
				appUI.Error("[%d/%d] Couldn't interact with contract: %s. Skip.", idx, total, err)
				results = append(results, batchResult{Index: idx, Network: n, Result: "Error"})
				continue
			}

			// Fetch status first — show compact summary for already-handled txs.
			_, confirmed, executed, summary := cmdutil.AnalyzeAndShowMsigTxInfo(appUI, multisigContract, txid, network, cmdutil.DefaultABIResolver{}, cm.Analyzer(network))
			if executed {
				appUI.Warn("Already executed — skipping.")
				results = append(results, batchResult{Index: idx, Network: n, Summary: summary, Result: "✓ Executed"})
				continue
			}
			if confirmed {
				appUI.Warn("Already confirmed — execute it instead. Skipping.")
				results = append(results, batchResult{Index: idx, Network: n, Summary: summary, Result: "Confirmed"})
				continue
			}

			from, err := GetApproverAccountFromMsig(multisigContract)
			if err != nil {
				appUI.Error("No approver wallet found. Skip.")
				results = append(results, batchResult{Index: idx, Network: n, Summary: summary, Result: "No wallet"})
				continue
			}

			data, err := a.Pack("confirmTransaction", txid)
			if err != nil {
				appUI.Error("Couldn't pack data: %s. Skip.", err)
				results = append(results, batchResult{Index: idx, Network: n, Summary: summary, Result: "Error"})
				continue
			}

			txType, err := cmdutil.ValidTxType(cm.Reader(network), network)
			if err != nil {
				appUI.Error("Couldn't determine tx type: %s", err)
				results = append(results, batchResult{Index: idx, Network: n, Summary: summary, Result: "Error"})
				return
			}

			if txType == types.LegacyTxType && config.TipGas > 0 {
				appUI.Warn("Legacy tx — ignoring tip gas parameter.")
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
						appUI.Warn("Skipped by user.")
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
				if errors.Is(err, ErrUserAborted) {
					results = append(results, batchResult{Index: idx, Network: n, Summary: summary, Result: "Skipped"})
				} else if errors.Is(err, ErrNotWaitingForMining) {
					results = append(results, batchResult{Index: idx, Network: n, Summary: summary, Result: "Broadcasted"})
				} else {
					appUI.Error("Failed to broadcast: %s. Aborted.", err)
					results = append(results, batchResult{Index: idx, Network: n, Summary: summary, Result: "Failed"})
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
			results = append(results, batchResult{Index: idx, Network: n, Summary: summary, Result: "Approved"})
		}

		// --- Batch summary table ---
		if len(results) > 1 {
			appUI.Section("Batch Summary")
			var rows [][]string
			for _, r := range results {
				action := r.Summary.Action
				if action == "" {
					action = "—"
				}
				amount := r.Summary.Amount
				if amount != "" {
					action = action + " " + amount
				}
				rows = append(rows, []string{
					fmt.Sprintf("%d", r.Index),
					r.Network,
					action,
					r.Result,
				})
			}
			appUI.Table([]string{"#", "Network", "Action", "Result"}, rows)
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
		tc, _ := cmdutil.TxContextFrom(cmd)

		reader := tc.Reader
		if reader == nil {
			appUI.Error("Couldn't connect to blockchain.")
			return
		}

		msigABI := util.GetGnosisMsigABI()

		cAddr := crypto.CreateAddress(jarviscommon.HexToAddress(tc.From), tc.Nonce).Hex()

		data, err := cmdutil.PromptTxData(
			appUI,
			tc.Analyzer,
			cAddr,
			cmdutil.CONSTRUCTOR_METHOD_INDEX,
			tc.PrefillParams,
			tc.PrefillMode,
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

		gasLimit := config.GasLimit
		if gasLimit == 0 {
			gasLimit, err = reader.EstimateExactGas(tc.From, "", 0, tc.Value, bytecode)
			if err != nil {
				appUI.Error("Couldn't estimate gas limit: %s", err)
				return
			}
		}
		tx := jarviscommon.BuildContractCreationTx(
			tc.TxType,
			tc.Nonce,
			tc.Value,
			gasLimit+config.ExtraGasLimit,
			tc.GasPrice+config.ExtraGasPrice,
			tc.TipGas+config.ExtraTipGas,
			bytecode,
			config.Network().GetChainID(),
		)

		if broadcasted, err := cmdutil.SignAndBroadcast(
			appUI, tc.FromAcc, tx, customABIs,
			reader, tc.Analyzer, nil, tc.Broadcaster,
		); err != nil && !broadcasted {
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

		tc, _ := cmdutil.TxContextFrom(cmd)
		var msigToName string
		config.MsigTo, msigToName, err = tc.Resolver.GetAddressFromString(config.MsigTo)
		if err != nil {
			return err
		}
		appUI.Info("Call to: %s (%s)", config.MsigTo, msigToName)
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)

		reader := tc.Reader
		if reader == nil {
			appUI.Error("Couldn't connect to blockchain.")
			return
		}

		a, err := tc.Resolver.ConfigToABI(config.MsigTo, config.ForceERC20ABI, config.CustomABI, config.Network())
		if err != nil {
			appUI.Warn("Couldn't get abi for %s: %s. Continue:", config.MsigTo, err)
		}

		data := []byte{}
		if a != nil && !config.NoFuncCall {
			data, err = cmdutil.PromptTxData(
				appUI,
				tc.Analyzer,
				config.MsigTo,
				config.MethodIndex,
				tc.PrefillParams,
				tc.PrefillMode,
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

		msigABI, err := tc.Resolver.GetABI(tc.To, config.Network())
		if err != nil {
			appUI.Error("Couldn't get the multisig's ABI: %s", err)
			return
		}

		if config.Simulate {
			multisigContract, err := msig.NewMultisigContract(tc.To, config.Network())
			if err != nil {
				appUI.Error("Couldn't interact with the contract: %s", err)
				return
			}
			err = multisigContract.SimulateSubmit(tc.From, config.MsigTo, jarviscommon.FloatToBigInt(config.MsigValue, config.Network().GetNativeTokenDecimal()), data)
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

		gasLimit := config.GasLimit
		if gasLimit == 0 {
			gasLimit, err = reader.EstimateExactGas(tc.From, tc.To, 0, tc.Value, txdata)
			if err != nil {
				appUI.Error("Couldn't estimate gas limit: %s", err)
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
			txdata,
			config.Network().GetChainID(),
		)

		customABIs := map[string]*abi.ABI{
			strings.ToLower(config.MsigTo): a,
			strings.ToLower(tc.To):         msigABI,
		}

		broadcasted, err := cmdutil.SignAndBroadcast(
			appUI, tc.FromAcc, tx, customABIs,
			reader, tc.Analyzer, a, tc.Broadcaster,
		)
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
