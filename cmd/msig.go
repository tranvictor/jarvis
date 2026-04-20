package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
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
	"github.com/tranvictor/jarvis/util/reader"
)

var ErrUserAborted = errors.New("user aborted")
var ErrNotWaitingForMining = errors.New("not waiting for mining")

var summaryMsigCmd = &cobra.Command{
	Use:   "summary",
	Short: "List pending Gnosis multisig transactions (Classic on-chain queue or Safe Transaction Service queue)",
	Long: `Inspects the multisig at args[0] and lists pending transactions.

Works against both Gnosis Classic (scans on-chain transaction ids) and
Gnosis Safe (queries the Safe Transaction Service), dispatched
automatically based on an on-chain probe of the address.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonMultisigReadPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		if tc.MultisigType == cmdutil.MultisigSafe {
			summarySafeCmd.Run(cmd, args)
			return
		}

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
	Short: "Show full detail of a single pending multisig transaction",
	Long: `Inspects the multisig at args[0] and prints the full detail of one
pending transaction (decoded calldata, signers, status). The second
argument identifies the tx: SafeTx hash / SafeTx nonce for Safe targets,
or msig tx id / init tx hash for Classic targets.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonMultisigReadPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		if tc.MultisigType == cmdutil.MultisigSafe {
			infoSafeCmd.Run(cmd, args)
			return
		}

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

		cmdutil.AnalyzeAndShowMsigTxInfo(appUI, multisigContract, txid, config.Network(), cmdutil.DefaultABIResolver{}, tc.Analyzer) //nolint:dogsled
	},
}

var govInfoMsigCmd = &cobra.Command{
	Use:   "gov",
	Short: "Show owners, threshold (and version, for Safe) of a Gnosis multisig",
	Long: `Prints the governance shape of the multisig at args[0]. For Gnosis
Safe targets this includes owners, threshold, version and on-chain Safe
nonce; for Gnosis Classic this includes owners, vote requirement and
the on-chain transaction count.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonMultisigReadPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		if tc.MultisigType == cmdutil.MultisigSafe {
			govSafeCmd.Run(cmd, args)
			return
		}

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
	Short: "Revoke a Gnosis Classic multisig confirmation (Classic only)",
	Long: `Revoke is a Gnosis Classic-specific operation: it sends an on-chain
revokeConfirmation(txid) call to undo a previously-given confirmation.
Gnosis Safe doesn't expose an equivalent — Safe approvals live in the
off-chain Safe Transaction Service; to "un-approve", just don't submit
your signature, or contact the proposer to delete the queued tx in the
Safe-app UI.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonMultisigTxPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		if tc.MultisigType == cmdutil.MultisigSafe {
			appUI.Error(
				"`revoke` is only available for Gnosis Classic multisigs. " +
					"For Gnosis Safe, simply do not submit your approval, or " +
					"ask the proposer to delete the queued tx in the Safe-app UI.",
			)
			return
		}
		cmdutil.HandleApproveOrRevokeOrExecuteMsig(appUI, "revokeConfirmation", cmd, args, nil)
	},
}

var executeMsigCmd = &cobra.Command{
	Use:   "execute",
	Short: "Execute a confirmed multisig transaction (Classic execTransaction or Safe execTransaction)",
	Long: `Broadcast the on-chain execution of a multisig transaction whose
confirmations meet the threshold. Classic targets call
executeTransaction(txid); Safe targets call execTransaction(...) with
the off-chain-collected signatures assembled from the Safe Transaction
Service.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonMultisigTxPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		if tc.MultisigType == cmdutil.MultisigSafe {
			executeSafeCmd.Run(cmd, args)
			return
		}
		cmdutil.HandleApproveOrRevokeOrExecuteMsig(appUI, "executeTransaction", cmd, args, nil)
	},
}

var approveMsigCmd = &cobra.Command{
	Use:   "approve",
	Short: "Approve a pending multisig transaction (Classic confirmTransaction or Safe off-chain/on-chain confirm)",
	Long: `Add your approval to a pending multisig transaction. For Gnosis
Classic this sends an on-chain confirmTransaction(txid); for Gnosis
Safe this signs the EIP-712 safeTxHash and posts the signature to the
Safe Transaction Service. For Safe targets, when your approval brings
the signature count to or above the Safe's threshold, jarvis chains
the on-chain execTransaction in the same invocation unless --no-execute
is set.

For Safe, pass --approve-onchain to approve via Safe.approveHash(...) on
chain rather than posting an EIP-712 signature to the Safe Transaction
Service. On-chain and off-chain approvals are merged transparently at
execution time, so the two modes can be mixed freely across signers.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonMultisigTxPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		if tc.MultisigType == cmdutil.MultisigSafe {
			approveSafeCmd.Run(cmd, args)
			return
		}
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

type msigTxConfirmation struct {
	sender string
	txHash string
}

type msigTxHistory struct {
	confirmations   []msigTxConfirmation
	executionTxHash string
}

const logsChunkSize = 9000

func clearProgressLine() {
	fmt.Fprintf(os.Stdout, "\r%s\r", strings.Repeat(" ", 80))
}

func queryMsigTxHistory(ethReader *reader.EthReader, msigABI *abi.ABI, msigAddr string, txID *big.Int, fromBlock int64, network jarvisnetworks.Network, executed bool, numConfirmations int) *msigTxHistory {
	history := &msigTxHistory{}
	txIDHash := ethcommon.BigToHash(txID)

	currentBlock, err := ethReader.CurrentBlock()
	if err != nil {
		appUI.Warn("Couldn't get current block: %s", err)
		return history
	}
	latestBlock := int64(currentBlock)
	totalBlocks := latestBlock - fromBlock + 1

	confirmTopic := msigABI.Events["Confirmation"].ID.Hex()
	addrs := []string{msigAddr}

	// Phase 1: find all Confirmation events, stop as soon as we have enough
	for start := fromBlock; start <= latestBlock && len(history.confirmations) < numConfirmations; start += logsChunkSize {
		end := start + logsChunkSize - 1
		if end > latestBlock {
			end = latestBlock
		}

		pct := int(float64(start-fromBlock) / float64(totalBlocks) * 100)
		fmt.Fprintf(os.Stdout, "\r  Querying confirmation logs... %d%%", pct)

		logs, err := ethReader.GetLogs(int(start), int(end), addrs, confirmTopic)
		if err != nil {
			clearProgressLine()
			appUI.Warn("Couldn't query confirmation logs: %s", err)
			return history
		}
		for _, l := range logs {
			if len(l.Topics) >= 3 && l.Topics[2] == txIDHash {
				sender := ethcommon.BytesToAddress(l.Topics[1].Bytes()).Hex()
				history.confirmations = append(history.confirmations, msigTxConfirmation{
					sender: sender,
					txHash: l.TxHash.Hex(),
				})
			}
		}

		time.Sleep(200 * time.Millisecond)
	}
	clearProgressLine()

	// Phase 2: find the Execution event if the tx is executed.
	// Execution can only happen at or after the last confirmation, so
	// start from that block instead of scanning from the beginning.
	if executed {
		execFrom := fromBlock
		if n := len(history.confirmations); n > 0 {
			lastConfTx := history.confirmations[n-1].txHash
			txinfo, err := ethReader.TxInfoFromHash(lastConfTx)
			if err == nil && txinfo.Receipt != nil {
				execFrom = txinfo.Receipt.BlockNumber.Int64()
			}
		}

		execTopic := msigABI.Events["Execution"].ID.Hex()
		execTotal := latestBlock - execFrom + 1

		for start := execFrom; start <= latestBlock && history.executionTxHash == ""; start += logsChunkSize {
			end := start + logsChunkSize - 1
			if end > latestBlock {
				end = latestBlock
			}

			pct := int(float64(start-execFrom) / float64(execTotal) * 100)
			fmt.Fprintf(os.Stdout, "\r  Querying execution logs... %d%%", pct)

			execLogs, err := ethReader.GetLogs(int(start), int(end), addrs, execTopic)
			if err != nil {
				clearProgressLine()
				appUI.Warn("Couldn't query execution logs: %s", err)
				return history
			}
			for _, l := range execLogs {
				if len(l.Topics) >= 2 && l.Topics[1] == txIDHash {
					history.executionTxHash = l.TxHash.Hex()
					break
				}
			}

			time.Sleep(200 * time.Millisecond)
		}
		clearProgressLine()
	}

	return history
}

type batchResult struct {
	network       string
	networkObj    jarvisnetworks.Network
	initTxHash    string
	confirmTxHash string
	msigTxID      string
	status        string // "approved", "broadcasted", "skipped", "failed"
	reason        string
	history       *msigTxHistory
}

func printBatchSummary(results []batchResult) {
	appUI.Section("Batch Approve Summary")
	approved, broadcasted, skipped, failed := 0, 0, 0, 0
	for i, r := range results {
		msigLabel := ""
		if r.msigTxID != "" {
			msigLabel = fmt.Sprintf(" (msig #%s)", r.msigTxID)
		}

		switch r.status {
		case "approved":
			approved++
			appUI.Success("  %d. [%s]%s — approved", i+1, r.network, msigLabel)
		case "broadcasted":
			broadcasted++
			appUI.Success("  %d. [%s]%s — broadcasted (not waiting for mining)", i+1, r.network, msigLabel)
		case "skipped":
			skipped++
			appUI.Warn("  %d. [%s]%s — skipped: %s", i+1, r.network, msigLabel, r.reason)
		case "failed":
			failed++
			appUI.Error("  %d. [%s]%s — failed: %s", i+1, r.network, msigLabel, r.reason)
		}

		if r.history != nil && len(r.history.confirmations) > 0 {
			for j, c := range r.history.confirmations {
				tag := "confirm"
				if j == 0 {
					tag = "init   "
				}
				senderName := c.sender
				if r.networkObj != nil {
					addr := util.GetJarvisAddress(c.sender, r.networkObj)
					senderName = appUI.Style(util.StyledAddress(addr))
				}
				appUI.Info("       %s tx: %s (by %s)", tag, c.txHash, senderName)
			}
			if r.confirmTxHash != "" {
				found := false
				for _, c := range r.history.confirmations {
					if strings.EqualFold(c.txHash, r.confirmTxHash) {
						found = true
						break
					}
				}
				if !found {
					appUI.Info("       confirm tx: %s (pending)", r.confirmTxHash)
				}
			}
			if r.history.executionTxHash != "" {
				appUI.Info("       exec    tx: %s", r.history.executionTxHash)
			}
		} else {
			if r.initTxHash != "" {
				appUI.Info("       init tx: %s", r.initTxHash)
			}
			if r.confirmTxHash != "" {
				appUI.Info("       approve tx: %s", r.confirmTxHash)
			}
		}
	}
	appUI.Info("")
	parts := []string{}
	if approved > 0 {
		parts = append(parts, fmt.Sprintf("%d approved", approved))
	}
	if broadcasted > 0 {
		parts = append(parts, fmt.Sprintf("%d broadcasted", broadcasted))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	appUI.Info("Total: %d transactions (%s)", len(results), strings.Join(parts, ", "))
}

type jsonConfirmation struct {
	TxHash string `json:"tx_hash"`
	Sender string `json:"sender"`
}

type jsonBatchResult struct {
	Network       string             `json:"network"`
	MsigTxID      string             `json:"msig_tx_id,omitempty"`
	Status        string             `json:"status"`
	Reason        string             `json:"reason,omitempty"`
	InitTxHash    string             `json:"init_tx_hash,omitempty"`
	ConfirmTxHash string             `json:"confirm_tx_hash,omitempty"`
	Confirmations []jsonConfirmation `json:"confirmations,omitempty"`
	ExecutionTx   string             `json:"execution_tx,omitempty"`
}

type jsonBatchSummary struct {
	Total       int               `json:"total"`
	Approved    int               `json:"approved"`
	Broadcasted int               `json:"broadcasted"`
	Skipped     int               `json:"skipped"`
	Failed      int               `json:"failed"`
	Results     []jsonBatchResult `json:"results"`
}

func writeBatchSummaryJSON(path string, results []batchResult) {
	summary := jsonBatchSummary{
		Total:   len(results),
		Results: make([]jsonBatchResult, 0, len(results)),
	}

	for _, r := range results {
		jr := jsonBatchResult{
			Network:       r.network,
			MsigTxID:      r.msigTxID,
			Status:        r.status,
			Reason:        r.reason,
			InitTxHash:    r.initTxHash,
			ConfirmTxHash: r.confirmTxHash,
		}

		if r.history != nil {
			for _, c := range r.history.confirmations {
				jr.Confirmations = append(jr.Confirmations, jsonConfirmation{
					TxHash: c.txHash,
					Sender: c.sender,
				})
			}
			jr.ExecutionTx = r.history.executionTxHash
		}

		summary.Results = append(summary.Results, jr)

		switch r.status {
		case "approved":
			summary.Approved++
		case "broadcasted":
			summary.Broadcasted++
		case "skipped":
			summary.Skipped++
		case "failed":
			summary.Failed++
		}
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		appUI.Error("Couldn't marshal JSON: %s", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		appUI.Error("Couldn't write JSON file: %s", err)
		return
	}
	appUI.Success("Summary written to %s", path)
}

var batchApproveMsigCmd = &cobra.Command{
	Use:   "bapprove",
	Short: "Approve a mixed batch of pending Classic and Safe multisig transactions",
	Long: `Process a list of multisig transaction references and approve each.
Each whitespace- or comma-separated token may be:

  - a Gnosis Classic init tx hash, optionally network-prefixed:
      mainnet:0x<64-hex>   or   bsc 0x<64-hex>

  - a Gnosis Safe app URL:
      https://app.safe.global/transactions/tx?id=multisig_<safe>_<hash>&safe=<chain>:<safe>

  - a bare Safe multisig token: multisig_<safe>_<hash>

  - an EIP-3770 Safe triple: <chain>:<safe>:<hash>

Safe references are extracted first; remaining tokens are treated as
Gnosis Classic init tx hashes. A summary table is printed at the end;
with --json-output, the same data is also written as JSON.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			appUI.Error("Please provide one or more multisig tx references.")
			return
		}
		raw := strings.Join(args, " ")

		safeRefs := scanForSafeRefs(raw)
		residual := raw
		for _, sr := range safeRefs {
			residual = strings.ReplaceAll(residual, sr.original, " ")
		}

		// Process the Safe portion of the input first so any auto-execute
		// flow runs before we start touching Classic broadcasters that may
		// share a wallet.
		if len(safeRefs) > 0 {
			total := len(safeRefs)
			results := make([]safeBatchResult, 0, total)
			appUI.Section(fmt.Sprintf("Batch Approve (Safe): %d transactions", total))
			for i, ref := range safeRefs {
				r := safeBatchResult{ref: ref.original}
				appUI.Info("")
				appUI.Critical("━━━ Safe [%d/%d] %s ━━━", i+1, total, ref.original)
				res := approveSafeRef(ref)
				r.network = res.network
				r.networkObj = res.networkObj
				r.safeAddress = res.safeAddress
				r.safeTxHash = res.safeTxHash
				r.confirmType = res.confirmType
				r.execTxHash = res.execTxHash
				r.status = res.status
				r.reason = res.reason
				results = append(results, r)
			}
			printSafeBatchSummary(results)
			if config.JSONOutputFile != "" {
				writeSafeBatchSummaryJSON(config.JSONOutputFile, results)
			}
		}

		networkNames, txs := cmdutil.ScanForTxs(residual)
		if len(networkNames) == 0 || len(txs) == 0 {
			if len(safeRefs) == 0 {
				appUI.Error("No txs passed to the first param. Did nothing.")
			}
			return
		}

		cm := walletarmy.NewWalletManager()
		a := util.GetGnosisMsigABI()

		total := len(networkNames)
		results := make([]batchResult, 0, total)

		appUI.Section(fmt.Sprintf("Batch Approve (Classic): %d transactions", total))

		for i, n := range networkNames {
			txHash := txs[i]
			r := batchResult{network: n, initTxHash: txHash}

			appUI.Info("")
			appUI.Critical("━━━ Classic [%d/%d] %s: %s ━━━", i+1, total, n, txHash)

			network, err := jarvisnetworks.GetNetwork(n)
			if err != nil {
				appUI.Error("%s network is not supported. Skip.", n)
				r.status = "skipped"
				r.reason = "unsupported network"
				results = append(results, r)
				continue
			}
			r.networkObj = network
			txinfo, err := cm.Reader(network).TxInfoFromHash(txHash)
			if err != nil {
				appUI.Error("Couldn't get tx info from hash: %s. Skip.", err)
				r.status = "failed"
				r.reason = fmt.Sprintf("tx info: %s", err)
				results = append(results, r)
				continue
			}
			if txinfo.Receipt == nil {
				appUI.Warn("This tx is still pending. Skip.")
				r.status = "skipped"
				r.reason = "tx still pending"
				results = append(results, r)
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
				appUI.Warn("This tx is not a gnosis classic multisig init tx. Skip.")
				r.status = "skipped"
				r.reason = "not a gnosis init tx"
				results = append(results, r)
				continue
			}

			r.msigTxID = txid.String()
			initBlock := txinfo.Receipt.BlockNumber.Int64()

			multisigContract, err := msig.NewMultisigContract(msigHex, network)
			if err != nil {
				appUI.Error("Couldn't interact with the contract: %s. Skip.", err)
				r.status = "failed"
				r.reason = fmt.Sprintf("contract: %s", err)
				results = append(results, r)
				continue
			}

			from, err := GetApproverAccountFromMsig(multisigContract)
			if err != nil {
				appUI.Error("Couldn't read and get wallet to approve this msig. You might not have any approver wallets.")
				r.status = "failed"
				r.reason = "no approver wallet"
				results = append(results, r)
				continue
			}

			_, numConf, confirmed, executed := cmdutil.AnalyzeAndShowMsigTxInfo(appUI, multisigContract, txid, network, cmdutil.DefaultABIResolver{}, cm.Analyzer(network))
			if executed {
				appUI.Warn("Already executed. Skip.")
				r.history = queryMsigTxHistory(cm.Reader(network), a, msigHex, txid, initBlock, network, true, numConf)
				r.status = "skipped"
				r.reason = "already executed"
				results = append(results, r)
				continue
			}
			if confirmed {
				appUI.Warn("Already confirmed but not executed. Consider executing instead. Skip.")
				r.history = queryMsigTxHistory(cm.Reader(network), a, msigHex, txid, initBlock, network, false, numConf)
				r.status = "skipped"
				r.reason = "already confirmed"
				results = append(results, r)
				continue
			}

			data, err := a.Pack("confirmTransaction", txid)
			if err != nil {
				appUI.Error("Couldn't pack data: %s. Skip.", err)
				r.status = "failed"
				r.reason = fmt.Sprintf("pack data: %s", err)
				results = append(results, r)
				continue
			}

			txType, err := cmdutil.ValidTxType(cm.Reader(network), network)
			if err != nil {
				appUI.Error("Couldn't determine proper tx type: %s. Aborting.", err)
				r.status = "failed"
				r.reason = fmt.Sprintf("tx type: %s", err)
				results = append(results, r)
				printBatchSummary(results)
				if config.JSONOutputFile != "" {
					writeBatchSummaryJSON(config.JSONOutputFile, results)
				}
				return
			}

			if txType == types.LegacyTxType && config.TipGas > 0 {
				appUI.Warn("Legacy tx — ignoring tip gas parameter.")
			}

			var confirmHash string
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
						appUI.Warn("User skipped. Continue with next tx.")
						return fmt.Errorf("%w: %w", ErrUserAborted, err)
					}
					return nil
				},
				func(broadcastedTx *types.Transaction, signError error) error {
					if signError != nil {
						return signError
					}
					if broadcastedTx != nil {
						confirmHash = broadcastedTx.Hash().Hex()
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

			r.confirmTxHash = confirmHash
			if err != nil {
				if errors.Is(err, ErrUserAborted) {
					r.status = "skipped"
					r.reason = "user aborted"
				} else if errors.Is(err, ErrNotWaitingForMining) {
					r.status = "broadcasted"
					r.history = queryMsigTxHistory(cm.Reader(network), a, msigHex, txid, initBlock, network, false, numConf)
				} else {
					appUI.Error("Failed to broadcast the tx after retries: %s.", err)
					r.status = "failed"
					r.reason = fmt.Sprintf("broadcast: %s", err)
				}
				results = append(results, r)
				continue
			}

			r.confirmTxHash = minedTx.Hash().Hex()
			if !config.DontWaitToBeMined {
				util.AnalyzeAndPrint(
					appUI,
					cm.Reader(network), cm.Analyzer(network),
					minedTx.Hash().Hex(), network, false, "", a, nil, config.DegenMode,
				)
			}

			r.history = queryMsigTxHistory(cm.Reader(network), a, msigHex, txid, initBlock, network, true, numConf+1)
			r.status = "approved"
			results = append(results, r)
		}

		printBatchSummary(results)
		if config.JSONOutputFile != "" {
			writeBatchSummaryJSON(config.JSONOutputFile, results)
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
	Short: "Propose a new multisig transaction (Classic submitTransaction or Safe off-chain proposal)",
	Long: `Propose a new multisig transaction targeting --msig-to with calldata
built interactively from the target's ABI. For Gnosis Classic this
sends an on-chain submitTransaction(...); for Gnosis Safe this signs
the EIP-712 safeTxHash and posts the proposal to the Safe Transaction
Service so other owners can approve it.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		if err = cmdutil.CommonMultisigTxPreprocess(appUI, cmd, args); err != nil {
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
		if tc.MultisigType == cmdutil.MultisigSafe {
			initSafeCmd.Run(cmd, args)
			return
		}

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
	initMsigCmd.Flags().BoolVarP(&config.Simulate, "simulate", "S", false, "True: Simulate execution of underlying call (Classic only).")
	initMsigCmd.Flags().Uint64Var(&safeNonceOverride, "safe-nonce", 0, "Safe-only: override the SafeTx nonce. Default: on-chain nonce + length of pending queue.")
	initMsigCmd.Flags().StringVar(&safeTxFile, "safe-tx-file", "", "Safe-only: path to a local file to write the proposed SafeTx + signature to. When set, jarvis skips the Safe Transaction Service and treats the file as the source of truth for later approve/execute runs.")
	initMsigCmd.MarkFlagRequired("msig-to")

	approveMsigCmd.Flags().BoolVar(&safeNoExecute, "no-execute", false, "Safe-only: don't auto-execute even when this approval reaches the threshold.")
	approveMsigCmd.Flags().BoolVar(&safeApproveOnChain, "approve-onchain", false, "Safe-only: approve via Safe.approveHash(safeTxHash) on-chain instead of submitting an EIP-712 signature to the Safe Transaction Service.")
	approveMsigCmd.Flags().StringVar(&safeTxFile, "safe-tx-file", "", "Safe-only: path to a local Safe tx file (as produced by 'jarvis msig init --safe-tx-file'). When set, jarvis uses the file as the pending-tx source and appends the new off-chain signature to it instead of the Safe Transaction Service.")
	executeMsigCmd.Flags().StringVar(&safeTxFile, "safe-tx-file", "", "Safe-only: path to a local Safe tx file to execute. When set, jarvis loads the SafeTx and collected signatures from the file instead of the Safe Transaction Service.")
	transactionInfoMsigCmd.Flags().StringVar(&safeTxFile, "safe-tx-file", "", "Safe-only: path to a local Safe tx file to inspect. When set, jarvis reads the SafeTx and signatures from the file instead of the Safe Transaction Service.")
	batchApproveMsigCmd.PersistentFlags().BoolVar(&safeApproveOnChain, "approve-onchain", false, "Safe-only: approve each Safe ref via Safe.approveHash on-chain instead of the Safe Transaction Service.")

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
	batchApproveMsigCmd.PersistentFlags().StringVarP(&config.JSONOutputFile, "json-output", "o", "", "Write batch summary to a JSON file")

	msigCmd.AddCommand(approveMsigCmd)
	msigCmd.AddCommand(batchApproveMsigCmd)
	msigCmd.AddCommand(revokeMsigCmd)
	msigCmd.AddCommand(initMsigCmd)
	msigCmd.AddCommand(executeMsigCmd)
	msigCmd.AddCommand(newMsigCmd)
	rootCmd.AddCommand(msigCmd)
}
