package cmd

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/util"
)

// safeNonceOverride is a v1-only optional override for the SafeTx nonce.
// When zero, jarvis auto-detects the next available SafeTx nonce by combining
// the on-chain nonce with the pending queue from the Safe Transaction Service.
var safeNonceOverride uint64

// safeNoExecute disables the convenience auto-execute behavior of
// `jarvis msig approve`. By default, when an approval brings the signature
// count to or above the Safe's threshold, jarvis chains an execute in the
// same invocation so the last signer doesn't have to run a second command.
// Setting --no-execute keeps the legacy "approve only" behavior.
var safeNoExecute bool

// safeApproveOnChain switches `jarvis msig approve` from the off-chain
// default (sign EIP-712 safeTxHash + POST to Safe Transaction Service) to
// the on-chain path (call Safe.approveHash(safeTxHash) from --from). This
// is useful on chains without a Transaction Service, for contract signers
// that can't produce EIP-712 signatures, or for operators who prefer an
// on-chain audit trail over an off-chain signature store.
var safeApproveOnChain bool

// safeTxFile is the path to a local file used to serialize/deserialize a
// pending Safe transaction (SafeTx + collected signatures). Useful for
// chains without a Safe Transaction Service, or for air-gapped / offline
// signing workflows where signers pass a file around instead of posting
// to a shared service. For `init`: jarvis writes the proposal + the first
// signature. For `approve` (off-chain): jarvis reads the file, appends
// the new signature, and writes it back. For `execute`: jarvis reads the
// file and broadcasts execTransaction. When this flag is set, jarvis
// treats the file as the source of truth and does NOT consult the Safe
// Transaction Service, even if one is configured for the chain.
var safeTxFile string

// initSafeCmd is the Safe-specific implementation of `jarvis msig init`.
// It is no longer registered as its own cobra command: cmd/msig.go reads
// initSafeCmd.Run / .PersistentPreRunE and invokes them after the unified
// preprocess detects a Safe target. We keep the cobra wrapper (rather
// than splitting Run into a free function) so flags, Long descriptions
// and the existing in-Run TxContextFrom calls stay untouched.
var initSafeCmd = &cobra.Command{
	Use:   "init",
	Short: "Propose a new Safe transaction (off-chain via Safe Transaction Service)",
	Long: `Build a SafeTx targeting --msig-to with the call data interactively
constructed from the target's ABI, sign the EIP-712 safeTxHash with --from
(or the only owner you have a wallet for), and submit the proposal to the
Safe Transaction Service. Other owners can later approve via 'jarvis msig
approve' and any owner can finalise via 'jarvis msig execute'.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		if err = cmdutil.CommonSafeTxPreprocess(appUI, cmd, args); err != nil {
			return err
		}
		if config.MsigValue < 0 {
			return fmt.Errorf("safe value can't be negative")
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
		safeContract := tc.Safe

		appUI.Section("Safe info")
		showSafeInfo(safeContract)

		// txTo/txValue/txData/txOp are the four SafeTx fields the two input
		// modes disagree on; everything after this point is shared.
		var (
			txTo       string
			txValue    *big.Int
			txData     []byte
			txOp       = safe.OpCall
			batchABIs  map[string]*abi.ABI
			batchLabel string
		)

		if txBuilderBatch != nil {
			if err := assertTxBuilderSafeMatches(txBuilderBatch, safeContract.Address); err != nil {
				appUI.Error("%s", err)
				return
			}
			txTo, txValue, txData, txOp, batchABIs, batchLabel = buildTxBuilderSafeTx(tc)
			if txData == nil {
				return // buildTxBuilderSafeTx already reported the failure
			}
		} else {
			targetABI, err := tc.Resolver.ConfigToABI(
				config.MsigTo, config.ForceERC20ABI, config.CustomABI, config.Network(),
			)
			if err != nil {
				appUI.Warn("Couldn't get abi for %s: %s. Continue:", config.MsigTo, err)
			}

			callData := []byte{}
			if targetABI != nil && !config.NoFuncCall {
				callData, err = cmdutil.PromptTxData(
					appUI,
					tc.Analyzer,
					config.MsigTo,
					config.MethodIndex,
					tc.PrefillParams,
					tc.PrefillMode,
					targetABI,
					nil,
					config.Network(),
				)
				if err != nil {
					appUI.Error("Couldn't pack target call data: %s", err)
					appUI.Warn("Continue with EMPTY CALLING DATA")
					callData = []byte{}
				}
			}
			txTo = config.MsigTo
			txValue = jarviscommon.FloatToBigInt(
				config.MsigValue, config.Network().GetNativeTokenDecimal(),
			)
			txData = callData
		}

		if safeTxFile == "" && tc.Collector == nil {
			appUI.Error(
				"Safe Transaction Service is not available for chain %d, and --safe-tx-file was not set.",
				config.Network().GetChainID(),
			)
			appUI.Info("Either configure SAFE_TX_SERVICE_URL_%d to point at a self-hosted deployment,", config.Network().GetChainID())
			appUI.Info("or re-run with --safe-tx-file <path> to write the proposal to a local file.")
			return
		}

		safeNonce, err := nextSafeNonce(safeContract, tc.Collector)
		if err != nil {
			appUI.Error("Couldn't determine the next safe nonce: %s", err)
			return
		}
		appUI.Info("SafeTx nonce: %d", safeNonce)

		domainSep, err := safeContract.DomainSeparator()
		if err != nil {
			appUI.Error("Couldn't read on-chain domainSeparator: %s", err)
			return
		}

		stx := safe.NewSafeTx(
			ethcommon.HexToAddress(txTo),
			txValue,
			txData,
			txOp,
			safeNonce,
		)
		hash := stx.SafeTxHash(domainSep)

		if batchLabel != "" {
			appUI.Info("MultiSend   : %s", batchLabel)
		}
		showSafeTxToConfirmWithABIs(stx, hash, &tc, batchABIs)
		if !config.YesToAllPrompt && !appUI.Confirm("Sign and submit this Safe transaction?", true) {
			appUI.Warn("Aborted by user.")
			return
		}

		appUI.Info("Unlock your wallet and sign the EIP-712 safeTxHash now...")
		sig, err := signSafeTx(tc.FromAcc, stx, domainSep)
		if errors.Is(err, errSignSafeHash) {
			appUI.Error("Couldn't sign safeTxHash: %s", signCause(err))
			return
		}
		if err != nil {
			appUI.Error("Couldn't unlock wallet: %s", err)
			return
		}

		if err := safe.SubmitProposal(
			tc.Collector,
			safeTxFile,
			ethcommon.HexToAddress(safeContract.Address),
			config.Network().GetChainID(),
			stx, hash,
			ethcommon.HexToAddress(tc.From),
			sig,
		); err != nil {
			if safeTxFile != "" {
				appUI.Error("Couldn't write Safe tx file: %s", err)
			} else {
				appUI.Error("Submitting proposal to Safe Transaction Service failed: %s", err)
			}
			return
		}

		if safeTxFile != "" {
			appUI.Success("Proposal written to %s", safeTxFile)
			appUI.Info("network: %s (chain %d)", config.Network().GetName(), config.Network().GetChainID())
			appUI.Info("safeTxHash: 0x%s", ethcommon.Bytes2Hex(hash[:]))
			appUI.Info("Share the file with other owners; each can run:")
			appUI.Info("  jarvis msig approve %s --safe-tx-file %s%s", safeContract.Address, safeTxFile, networkFlag())
			appUI.Info("Once threshold is met, any owner can run:")
			appUI.Info("  jarvis msig execute %s --safe-tx-file %s%s", safeContract.Address, safeTxFile, networkFlag())
			return
		}
		appUI.Success("Proposal submitted.")
		appUI.Info("network: %s (chain %d)", config.Network().GetName(), config.Network().GetChainID())
		appUI.Info("safeTxHash: 0x%s", ethcommon.Bytes2Hex(hash[:]))
		appUI.Info("Other owners can approve with:")
		appUI.Info("  jarvis msig approve %s 0x%s%s", safeContract.Address, ethcommon.Bytes2Hex(hash[:]), networkFlag())
		appUI.Info("Once threshold is met, anyone can execute with:")
		appUI.Info("  jarvis msig execute %s 0x%s%s", safeContract.Address, ethcommon.Bytes2Hex(hash[:]), networkFlag())
	},
}

// approveSafeCmd is the Safe-specific implementation of `jarvis msig
// approve`. Dispatched from cmd/msig.go after the unified preprocess
// detects a Safe target and CommonSafeTxPreprocess has wired the
// Safe-specific TxContext fields. See initSafeCmd's docstring for why we
// keep the cobra wrapper rather than splitting Run into a free function.
var approveSafeCmd = &cobra.Command{
	Use:   "approve",
	Short: "Off-chain approve a pending Safe transaction (adds your signature to the service)",
	Long: `Sign the EIP-712 safeTxHash of a pending Safe transaction and
submit your signature to the Safe Transaction Service. Identify the
pending tx by:

  - a Safe-app URL (the easiest form for non-CLI signers):
      jarvis msig approve "https://app.safe.global/transactions/tx?id=multisig_<safe>_<hash>&safe=eth:<safe>"

  - the safe address followed by a safeTxHash or SafeTx nonce:
      jarvis msig approve <safe> <safeTxHash|nonce>

If your approval brings the signature count to or above the Safe's
threshold, jarvis automatically chains an execTransaction in the same
invocation so you don't have to run a second command. Pass --no-execute
to opt out (the typical use case is when you want a different EOA to
pay for execution gas).

By default jarvis signs off-chain: it produces an EIP-712 signature of
safeTxHash and POSTs it to the Safe Transaction Service. Pass
--approve-onchain to use the on-chain path instead — jarvis will send a
Safe.approveHash(safeTxHash) transaction from --from. This mode is
useful on chains without a Transaction Service, for wallets that can't
produce EIP-712 signatures, or when you prefer an on-chain audit trail
over an off-chain signature store. Other owners' off-chain signatures
(and other owners' on-chain approvals) are merged at execution time.
`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("usage: jarvis msig approve <safe-or-url> [safeTxHash|nonce]")
		}
		return cmdutil.CommonSafeTxPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		safeContract := tc.Safe

		appUI.Section("Safe info")
		showSafeInfo(safeContract)

		pending, err := loadPendingTx(tc, args, safeTxFile, safeApproveOnChain)
		if errors.Is(err, errPendingNoCollector) {
			appUI.Error(
				"Safe Transaction Service is not available for chain %d.",
				config.Network().GetChainID(),
			)
			appUI.Info("Pass --approve-onchain to approve via Safe.approveHash directly, or")
			appUI.Info("pass --safe-tx-file <path> to load the pending SafeTx from a local file.")
			return
		}
		if errors.Is(err, errPendingNeedHash) {
			appUI.Error(
				"--approve-onchain without a Safe Transaction Service requires an explicit safeTxHash (0x... 32 bytes) or a Safe-app URL that carries one.",
			)
			return
		}
		if err != nil {
			appUI.Error("%s", err)
			return
		}
		if pending.IsExecuted {
			appUI.Warn("This transaction has already been executed; nothing to approve.")
			return
		}

		domainSep, err := safeContract.DomainSeparator()
		if err != nil {
			appUI.Error("Couldn't read on-chain domainSeparator: %s", err)
			return
		}

		// When we have SafeTx fields in hand (file mode, or service mode),
		// verify the declared safeTxHash is what the on-chain domainSep
		// and SafeTx fields produce. In the service-less --approve-onchain
		// corner case above we only have the hash, so there's nothing to
		// cross-check — the Safe itself will reject a wrong hash at
		// approveHash / execute time.
		if expected, err := verifyPendingSafeTxHash(pending, domainSep); errors.Is(err, errPendingHashMismatch) {
			appUI.Error(
				"declared safeTxHash (0x%s) doesn't match locally recomputed hash (0x%s); refusing to sign",
				ethcommon.Bytes2Hex(pending.SafeTxHash[:]),
				ethcommon.Bytes2Hex(expected[:]),
			)
			return
		}

		// Merge on-chain approvals into the in-memory Sigs before the
		// self-signed check and display. This way we correctly recognise
		// owners who approved via approveHash (and may not appear in the
		// service's confirmation list) as having already signed.
		if _, err := safeContract.MergeOnChainApprovals(pending); err != nil {
			appUI.Warn("Couldn't merge on-chain approvals: %s", err)
		}

		if pending.SafeTx != nil {
			showSafeTxToConfirm(pending.SafeTx, pending.SafeTxHash, &tc)
		} else {
			appUI.Info("safeTxHash: 0x%s (no SafeTx body available; only approving the hash on-chain)", ethcommon.Bytes2Hex(pending.SafeTxHash[:]))
		}
		showSafeSigners("Existing signatures", pending.Sigs)

		me := ethcommon.HexToAddress(tc.From)
		if onChain, found := ownerAlreadySigned(pending, me); found {
			if onChain {
				appUI.Warn("You (%s) have already approved this transaction on-chain (approveHash).", me.Hex())
			} else {
				appUI.Warn("You (%s) have already signed this transaction off-chain.", me.Hex())
			}
			return
		}

		if safeApproveOnChain {
			runSafeApproveOnChain(tc, safeContract, pending, domainSep, me)
			return
		}

		if pending.SafeTx == nil {
			appUI.Error("Off-chain approval requires the full SafeTx body; either pass --safe-tx-file, configure the Safe Transaction Service, or use --approve-onchain.")
			return
		}

		if !config.YesToAllPrompt && !appUI.Confirm("Sign and submit your approval?", true) {
			appUI.Warn("Aborted by user.")
			return
		}

		appUI.Info("Unlock your wallet and sign the EIP-712 safeTxHash now...")
		sig, err := signPendingSafeTx(tc.FromAcc, pending, domainSep)
		if errors.Is(err, errSignSafeHash) {
			appUI.Error("Couldn't sign safeTxHash: %s", signCause(err))
			return
		}
		if err != nil {
			appUI.Error("Couldn't unlock wallet: %s", err)
			return
		}

		// Persist the new signature. In file mode we append to the file
		// (the collective source of truth); otherwise we POST to the Safe
		// Transaction Service.
		if err := persistApproval(tc, safeContract.Address, pending, me, sig, safeTxFile, config.Network().GetChainID()); err != nil {
			if safeTxFile != "" {
				appUI.Error("Couldn't write updated Safe tx file: %s", err)
			} else {
				appUI.Error("Submitting confirmation to Safe Transaction Service failed: %s", err)
			}
			return
		}
		if safeTxFile != "" {
			appUI.Success("Signature appended to %s", safeTxFile)
		} else {
			appUI.Success("Confirmation submitted.")
		}
		totalSigs := len(pending.Sigs) + 1
		appUI.Info("Total signatures now: %d", totalSigs)

		threshold, err := safeContract.Threshold()
		if err != nil {
			appUI.Warn("Couldn't read safe threshold post-approval: %s", err)
			return
		}
		nextCmdHint := fmt.Sprintf("  jarvis msig execute %s 0x%s%s", safeContract.Address, ethcommon.Bytes2Hex(pending.SafeTxHash[:]), networkFlag())
		if safeTxFile != "" {
			nextCmdHint = fmt.Sprintf("  jarvis msig execute %s --safe-tx-file %s%s", safeContract.Address, safeTxFile, networkFlag())
		}
		if uint64(totalSigs) < threshold {
			appUI.Info(
				"Need %d more approval(s). Once threshold is met, any owner can run:",
				threshold-uint64(totalSigs),
			)
			appUI.Info("%s", nextCmdHint)
			return
		}

		// Threshold reached on this very approval. Unless the caller asked
		// us to stop here (--no-execute), chain straight into execTransaction
		// so the last signer doesn't need a second command. We use the
		// in-memory signature list (existing + ours) to avoid a race with
		// the Safe Transaction Service indexing our just-submitted sig.
		appUI.Success("Threshold (%d) met with this approval.", threshold)
		if safeNoExecute {
			appUI.Info("--no-execute set; skipping execTransaction. Run later with:")
			appUI.Info("%s", nextCmdHint)
			return
		}
		if !config.YesToAllPrompt && !appUI.Confirm("Broadcast execTransaction now?", true) {
			appUI.Warn("Skipping execution. Run later with:")
			appUI.Info("%s", nextCmdHint)
			return
		}

		runSafeExecute(tc, safeContract, pendingWithNewSig(pending, me, sig), domainSep)
	},
}

// runSafeApproveOnChain is the --approve-onchain code path for
// `jarvis msig approve`. It broadcasts a Safe.approveHash(safeTxHash)
// transaction from me and, if the resulting approval brings the set past
// threshold, chains an execTransaction in the same invocation just like
// the off-chain path does. The merge in runSafeExecute picks up the
// just-landed approval via approvedHashes(...) so we don't need to hand
// assemble signatures here.
//
// We deliberately do NOT short-circuit by synthesising a v=0 marker
// in-memory and jumping straight to execTransaction: a broadcast of
// approveHash may fail, or the user may have passed --no-wait, in which
// case executing immediately would revert on-chain with GS025. Running
// the merge after a successful mined approveHash keeps the two steps
// independent and correct.
func runSafeApproveOnChain(
	tc cmdutil.TxContext,
	safeContract *safe.SafeContract,
	pending *safe.PendingTx,
	domainSep [32]byte,
	me ethcommon.Address,
) {
	data, err := safeContract.Abi.Pack("approveHash", pending.SafeTxHash)
	if err != nil {
		appUI.Error("Couldn't pack approveHash calldata: %s", err)
		return
	}

	zeroValue := big.NewInt(0)
	gasLimit := config.GasLimit
	if gasLimit == 0 {
		gasLimit, err = tc.Reader.EstimateExactGas(tc.From, safeContract.Address, 0, zeroValue, data)
		if err != nil {
			appUI.Error("Couldn't estimate gas limit for approveHash: %s", err)
			return
		}
	}

	tx := jarviscommon.BuildExactTx(
		tc.TxType,
		tc.Nonce,
		safeContract.Address,
		zeroValue,
		gasLimit+config.ExtraGasLimit,
		tc.GasPrice+config.ExtraGasPrice,
		tc.TipGas+config.ExtraTipGas,
		data,
		config.Network().GetChainID(),
	)

	customABIs := map[string]*abi.ABI{
		strings.ToLower(safeContract.Address): safeContract.Abi,
	}

	appUI.Info("Broadcasting approveHash(0x%s) from %s...",
		ethcommon.Bytes2Hex(pending.SafeTxHash[:]), me.Hex(),
	)
	broadcasted, err := cmdutil.SignAndBroadcast(
		appUI, tc.FromAcc, tx, customABIs,
		tc.Reader, tc.Analyzer, safeContract.Abi, tc.Broadcaster,
	)
	if err != nil && !broadcasted {
		appUI.Error("approveHash failed: %s", err)
		return
	}
	if err != nil {
		appUI.Warn("approveHash was broadcast but post-processing reported: %s", err)
	}
	if !broadcasted {
		// --dont-broadcast path: signed blob was printed, nothing on chain.
		return
	}
	appUI.Success("approveHash broadcast.")

	// We can only safely chain an execute if the approveHash transaction
	// has been mined — otherwise approvedHashes(me, safeTxHash) is still
	// 0 and execTransaction would revert with GS025.
	if config.DontBroadcast || config.DontWaitToBeMined {
		appUI.Info("--no-wait / --dont-broadcast is in effect; skipping auto-execute.")
		appUI.Info("Once the approveHash tx is mined, finalise with:")
		appUI.Info(
			"  jarvis msig execute %s 0x%s%s",
			safeContract.Address, ethcommon.Bytes2Hex(pending.SafeTxHash[:]), networkFlag(),
		)
		return
	}

	// Re-read approvedHashes to confirm our approval actually landed. If
	// the node returned an early "mined" result that got re-orged out, or
	// an explorer-side caching quirk caused a false negative, we refuse
	// to execute rather than produce a guaranteed-revert transaction.
	v, err := safeContract.ApprovedHash(me.Hex(), pending.SafeTxHash)
	if err != nil {
		appUI.Warn("Couldn't confirm approveHash landed: %s", err)
		return
	}
	if v.Sign() == 0 {
		appUI.Warn("approveHash transaction was broadcast but approvedHashes(%s, ...) is still 0; skipping auto-execute.", me.Hex())
		return
	}
	pending.Sigs = append(pending.Sigs, safe.OnChainApprovalSig(me))

	threshold, err := safeContract.Threshold()
	if err != nil {
		appUI.Warn("Couldn't read safe threshold post-approval: %s", err)
		return
	}
	totalSigs := len(pending.Sigs)
	appUI.Info("Total signatures now: %d (threshold %d)", totalSigs, threshold)
	if uint64(totalSigs) < threshold {
		appUI.Info(
			"Need %d more approval(s). Once threshold is met, any owner can run:",
			threshold-uint64(totalSigs),
		)
		appUI.Info(
			"  jarvis msig execute %s 0x%s%s",
			safeContract.Address, ethcommon.Bytes2Hex(pending.SafeTxHash[:]), networkFlag(),
		)
		return
	}

	appUI.Success("Threshold (%d) met with this approval.", threshold)
	if safeNoExecute {
		appUI.Info("--no-execute set; skipping execTransaction. Run later with:")
		appUI.Info(
			"  jarvis msig execute %s 0x%s%s",
			safeContract.Address, ethcommon.Bytes2Hex(pending.SafeTxHash[:]), networkFlag(),
		)
		return
	}
	if !config.YesToAllPrompt && !appUI.Confirm("Broadcast execTransaction now?", true) {
		appUI.Warn("Skipping execution. Run later with:")
		appUI.Info(
			"  jarvis msig execute %s 0x%s%s",
			safeContract.Address, ethcommon.Bytes2Hex(pending.SafeTxHash[:]), networkFlag(),
		)
		return
	}

	// Important: the execTransaction below will consume the EOA nonce of
	// tc.From just like the approveHash we just sent. Since
	// cmdutil.CommonSafeTxPreprocess populated tc.Nonce once, at the top
	// of this command, it is now one behind reality. Re-read it so the
	// execute tx uses the correct nonce.
	if nextNonce, err := tc.Reader.GetMinedNonce(tc.From); err == nil {
		tc.Nonce = nextNonce
	} else {
		appUI.Warn("Couldn't refresh nonce before execute: %s", err)
	}

	runSafeExecute(tc, safeContract, pending, domainSep)
}

var executeSafeCmd = &cobra.Command{
	Use:   "execute",
	Short: "Execute a Safe transaction whose signatures meet the threshold",
	Long: `Fetch a pending Safe transaction, assemble its signatures into the
format Safe.execTransaction expects, and broadcast the on-chain execution
from --from (or the single matching owner you have a wallet for).

The pending tx can be identified by:

  - a Safe-app URL:
      jarvis msig execute "https://app.safe.global/transactions/tx?id=multisig_<safe>_<hash>&safe=eth:<safe>"

  - the safe address followed by a safeTxHash or SafeTx nonce:
      jarvis msig execute <safe> <safeTxHash|nonce>
`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("usage: jarvis msig execute <safe-or-url> [safeTxHash|nonce]")
		}
		return cmdutil.CommonSafeTxPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		safeContract := tc.Safe

		appUI.Section("Safe info")
		showSafeInfo(safeContract)

		pending, err := loadPendingTx(tc, args, safeTxFile, false)
		if errors.Is(err, errPendingNoCollector) {
			appUI.Error(
				"Safe Transaction Service is not available for chain %d, and --safe-tx-file was not set.",
				config.Network().GetChainID(),
			)
			appUI.Info("Pass --safe-tx-file <path> to execute from a local file, or configure SAFE_TX_SERVICE_URL_%d.", config.Network().GetChainID())
			return
		}
		if err != nil {
			appUI.Error("%s", err)
			return
		}
		if pending.IsExecuted {
			appUI.Warn("This transaction has already been executed.")
			return
		}

		domainSep, err := safeContract.DomainSeparator()
		if err != nil {
			appUI.Error("Couldn't read on-chain domainSeparator: %s", err)
			return
		}

		runSafeExecute(tc, safeContract, pending, domainSep)
	},
}

// summarySafeCmd lists every pending Safe transaction the Transaction
// Service knows about for a Safe, plus a short status line per entry. The
// classic-msig analogue scans every tx id on chain; here we ask the service
// because Safe doesn't number its txs sequentially on chain.
var summarySafeCmd = &cobra.Command{
	Use:   "summary",
	Short: "List all pending Safe transactions and their signature progress",
	Long: `Fetch the queue of pending (not-yet-executed) Safe transactions
from the Safe Transaction Service and print, for each one, the SafeTx
nonce, target, signature progress (n/threshold) and safeTxHash. Also
prints the on-chain Safe nonce so you can see how far ahead the queue is.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonSafeReadPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		safeContract := tc.Safe

		appUI.Section("Safe info")
		showSafeInfo(safeContract)

		if tc.Collector == nil {
			appUI.Error(
				"Safe Transaction Service is not available for chain %d; `summary` needs it to list the pending queue.",
				config.Network().GetChainID(),
			)
			appUI.Info("Configure SAFE_TX_SERVICE_URL_%d to point at a self-hosted service.", config.Network().GetChainID())
			return
		}
		threshold, _ := safeContract.Threshold()
		pending, err := tc.Collector.ListPending(ethcommon.HexToAddress(safeContract.Address))
		if err != nil {
			appUI.Error("Couldn't fetch the pending queue: %s", err)
			return
		}

		appUI.Section(fmt.Sprintf("Pending Safe transactions: %d", len(pending)))
		if len(pending) == 0 {
			appUI.Info("Queue is empty. Use `jarvis msig init` to propose a new tx.")
			return
		}
		for i, p := range pending {
			// Fold in on-chain approvals so the signature count reflects
			// reality rather than just the Safe Transaction Service view.
			// Errors here are non-fatal: we still want the queue listing.
			if _, err := safeContract.MergeOnChainApprovals(p); err != nil {
				appUI.Warn("  nonce %s: couldn't merge on-chain approvals (%s); count may be low", p.SafeTx.Nonce.String(), err)
			}
			toJarvis := util.GetJarvisAddress(p.SafeTx.To.Hex(), config.Network())
			progress := fmt.Sprintf("%d/%d", len(p.Sigs), threshold)
			status := "pending"
			switch {
			case p.IsExecuted:
				status = "executed"
			case threshold > 0 && uint64(len(p.Sigs)) >= threshold:
				status = "ready to execute"
			}
			appUI.Info(
				"  %d. nonce %s  sigs %s  status %s",
				i+1, p.SafeTx.Nonce.String(), progress, status,
			)
			appUI.Info("       to       %s", appUI.Style(util.StyledAddress(toJarvis)))
			appUI.Info("       safeTxHash 0x%s", ethcommon.Bytes2Hex(p.SafeTxHash[:]))
		}
	},
}

// infoSafeCmd shows the full detail (decoded calldata + signers list) of
// one pending Safe tx, identified the same way `safe approve` accepts it:
// by URL, by safeTxHash, or by SafeTx nonce.
var infoSafeCmd = &cobra.Command{
	Use:   "info",
	Short: "Show the full detail of one pending Safe transaction",
	Long: `Fetch a pending Safe transaction by safeTxHash, SafeTx nonce, or
Safe-app URL, and print its decoded calldata, signers and execution status.
Equivalent to ` + "`jarvis msig info`" + ` for Gnosis Classic.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonSafeReadPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		safeContract := tc.Safe

		appUI.Section("Safe info")
		showSafeInfo(safeContract)

		pending, err := loadPendingTx(tc, args, safeTxFile, false)
		if errors.Is(err, errPendingNoCollector) {
			appUI.Error(
				"Safe Transaction Service is not available for chain %d, and --safe-tx-file was not set.",
				config.Network().GetChainID(),
			)
			appUI.Info("Pass --safe-tx-file <path> to inspect a local Safe tx file, or configure SAFE_TX_SERVICE_URL_%d.", config.Network().GetChainID())
			return
		}
		if err != nil {
			appUI.Error("%s", err)
			return
		}

		if _, err := safeContract.MergeOnChainApprovals(pending); err != nil {
			appUI.Warn("Couldn't merge on-chain approvals: %s", err)
		}

		showSafeTxToConfirm(pending.SafeTx, pending.SafeTxHash, &tc)
		threshold, _ := safeContract.Threshold()
		showSafeSigners(
			fmt.Sprintf("Signatures (%d of %d required)", len(pending.Sigs), threshold),
			pending.Sigs,
		)
		switch {
		case pending.IsExecuted:
			appUI.Success("Status: executed.")
		case threshold > 0 && uint64(len(pending.Sigs)) >= threshold:
			appUI.Success("Status: threshold met — ready to execute.")
			appUI.Info(
				"  jarvis msig execute %s 0x%s%s",
				safeContract.Address, ethcommon.Bytes2Hex(pending.SafeTxHash[:]), networkFlag(),
			)
		default:
			needed := uint64(0)
			if threshold > uint64(len(pending.Sigs)) {
				needed = threshold - uint64(len(pending.Sigs))
			}
			appUI.Info("Status: pending — needs %d more approval(s).", needed)
			appUI.Info(
				"  jarvis msig approve %s 0x%s%s",
				safeContract.Address, ethcommon.Bytes2Hex(pending.SafeTxHash[:]), networkFlag(),
			)
		}
	},
}

// govSafeCmd prints owner list, threshold, version and on-chain nonce for
// a Safe. Read-only and equivalent to `jarvis msig gov` for the classic UI.
var govSafeCmd = &cobra.Command{
	Use:   "gov",
	Short: "Show owners, threshold, version and on-chain nonce of a Safe",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonSafeReadPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		appUI.Section("Safe governance")
		showSafeInfo(tc.Safe)
	},
}

// safeDeployPromptABI is the interactive shape of `jarvis msig new --type safe`.
// Factory / singleton / fallback-handler are resolved on-chain (or via
// overrides); the operator only supplies owners, threshold and CREATE2 salt.
const safeDeployPromptABI = `[
  {
    "name": "create",
    "type": "function",
    "stateMutability": "nonpayable",
    "inputs": [
      {"name": "_owners", "type": "address[]"},
      {"name": "_threshold", "type": "uint256"},
      {"name": "_saltNonce", "type": "uint256"}
    ]
  }
]`

// runNewSafe is the Safe path of `jarvis msig new`. It prompts for owners /
// threshold / salt, deploys a proxy via SafeProxyFactory.createProxyWithNonce,
// and prints the predicted CREATE2 address before the user signs.
func runNewSafe(cmd *cobra.Command, args []string) {
	tc, _ := cmdutil.TxContextFrom(cmd)
	if tc.Reader == nil {
		appUI.Error("Couldn't connect to blockchain.")
		return
	}

	dep, err := safe.ResolveSafeDeployment(config.Network(), safe.DeployOverrides{
		Factory:         msigNewFactory,
		Singleton:       msigNewSingleton,
		FallbackHandler: msigNewFallbackHandler,
	})
	if err != nil {
		appUI.Error("%s", err)
		return
	}

	promptABI, err := abi.JSON(strings.NewReader(safeDeployPromptABI))
	if err != nil {
		appUI.Error("Couldn't parse Safe deploy prompt ABI: %s", err)
		return
	}

	appUI.Section("New Gnosis Safe")
	appUI.Info("Factory          : %s (%s)", dep.Factory.Hex(), dep.FactoryLabel)
	appUI.Info("Singleton        : %s (%s)", dep.Singleton.Hex(), dep.SingletonLabel)
	appUI.Info("Fallback handler : %s (%s)", dep.FallbackHandler.Hex(), dep.FallbackLabel)
	appUI.Info("Salt nonce is any unused uint; the same owners + threshold + salt")
	appUI.Info("always produce the same Safe address (CREATE2). Use 0 unless you")
	appUI.Info("need a specific address or are retrying after a collision.")

	method, params, err := cmdutil.PromptFunctionCallData(
		appUI,
		tc.Analyzer,
		dep.Factory.Hex(),
		1,
		tc.PrefillParams,
		tc.PrefillMode,
		"write",
		&promptABI,
		nil,
		config.Network(),
	)
	if err != nil {
		appUI.Error("Couldn't read Safe deploy params: %s", err)
		return
	}
	_ = method

	owners, threshold, saltNonce, err := parseSafeDeployParams(params)
	if err != nil {
		appUI.Error("%s", err)
		return
	}
	if err := safe.ValidateNewSafeParams(owners, threshold); err != nil {
		appUI.Error("%s", err)
		return
	}

	initializer, err := safe.EncodeSetup(owners, threshold, dep.FallbackHandler)
	if err != nil {
		appUI.Error("Couldn't pack Safe.setup: %s", err)
		return
	}
	txData, err := safe.EncodeCreateProxyWithNonce(dep.Singleton, initializer, saltNonce)
	if err != nil {
		appUI.Error("Couldn't pack createProxyWithNonce: %s", err)
		return
	}

	var predicted ethcommon.Address
	var predictedOK bool
	if creation, err := safe.FactoryProxyCreationCode(dep.Factory, config.Network()); err != nil {
		appUI.Warn("Couldn't read factory.proxyCreationCode(); predicted address will be omitted: %s", err)
	} else if addr, err := safe.PredictProxyAddress(dep.Factory, dep.Singleton, creation, initializer, saltNonce); err != nil {
		appUI.Warn("Couldn't predict Safe address: %s", err)
	} else {
		predicted = addr
		predictedOK = true
		if occupied, err := util.IsContract(addr.Hex(), config.Network()); err == nil && occupied {
			appUI.Error(
				"Predicted Safe %s already has code. Pick a different salt nonce.",
				addr.Hex(),
			)
			return
		}
	}

	appUI.Section("Safe to be created")
	if predictedOK {
		appUI.Critical("Predicted address : %s", predicted.Hex())
	}
	appUI.Critical("Threshold         : %s / %d", threshold.String(), len(owners))
	appUI.Critical("Salt nonce        : %s", saltNonce.String())
	appUI.Info("Owners (%d):", len(owners))
	for i, o := range owners {
		jarvisAddr := util.GetJarvisAddress(o.Hex(), config.Network())
		appUI.Info("  %d. %s", i+1, appUI.Style(util.StyledAddress(jarvisAddr)))
	}

	gasLimit := config.GasLimit
	if gasLimit == 0 {
		gasLimit, err = tc.Reader.EstimateExactGas(tc.From, dep.Factory.Hex(), 0, tc.Value, txData)
		if err != nil {
			appUI.Error("Couldn't estimate gas limit for createProxyWithNonce: %s", err)
			return
		}
	}

	tx := jarviscommon.BuildExactTx(
		tc.TxType,
		tc.Nonce,
		dep.Factory.Hex(),
		tc.Value,
		gasLimit+config.ExtraGasLimit,
		tc.GasPrice+config.ExtraGasPrice,
		tc.TipGas+config.ExtraTipGas,
		txData,
		config.Network().GetChainID(),
	)

	customABIs := map[string]*abi.ABI{
		strings.ToLower(dep.Factory.Hex()): safe.GetProxyFactoryABI(),
	}

	if broadcasted, err := cmdutil.SignAndBroadcast(
		appUI, tc.FromAcc, tx, customABIs,
		tc.Reader, tc.Analyzer, safe.GetProxyFactoryABI(), tc.Broadcaster,
	); err != nil && !broadcasted {
		appUI.Error("Failed to proceed after signing the tx: %s. Aborted.", err)
		return
	}

	if predictedOK {
		appUI.Info("")
		appUI.Info("Next (after the deploy tx confirms):")
		appUI.Info("  jarvis msig gov %s%s", predicted.Hex(), networkFlag())
		appUI.Info("  jarvis msig init %s --msig-to <target>%s", predicted.Hex(), networkFlag())
	}
}

func parseSafeDeployParams(params []any) (owners []ethcommon.Address, threshold, saltNonce *big.Int, err error) {
	if len(params) != 3 {
		return nil, nil, nil, fmt.Errorf("expected 3 deploy params (owners, threshold, salt nonce), got %d", len(params))
	}
	owners, err = asAddressSlice(params[0])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("owners: %w", err)
	}
	threshold, err = asBigInt(params[1])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("threshold: %w", err)
	}
	saltNonce, err = asBigInt(params[2])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("salt nonce: %w", err)
	}
	return owners, threshold, saltNonce, nil
}

func asAddressSlice(v any) ([]ethcommon.Address, error) {
	if t, ok := v.([]ethcommon.Address); ok {
		return t, nil
	}
	return nil, fmt.Errorf("got %T, want []address", v)
}

func asBigInt(v any) (*big.Int, error) {
	switch t := v.(type) {
	case *big.Int:
		return t, nil
	case big.Int:
		n := t
		return &n, nil
	case uint64:
		return new(big.Int).SetUint64(t), nil
	case int64:
		return big.NewInt(t), nil
	case int:
		return big.NewInt(int64(t)), nil
	}
	return nil, fmt.Errorf("got %T, want integer", v)
}

// runSafeExecute drives the on-chain execTransaction call for a Safe
// transaction whose signatures have already been collected. It is shared
// between `jarvis msig execute` and the auto-execute path of `jarvis msig
// approve` so both flows enforce the same threshold check, hash verification,
// gas estimation and broadcast confirmation. domainSep is taken as an
// argument because the approve-then-execute path has already paid for that
// on-chain read and there's no reason to repeat it.
func runSafeExecute(
	tc cmdutil.TxContext,
	safeContract *safe.SafeContract,
	pending *safe.PendingTx,
	domainSep [32]byte,
) {
	// Before counting signatures, pull in any on-chain approvals the Safe
	// Transaction Service doesn't know about (owners who invoked
	// approveHash directly). Without this step we'd refuse to execute
	// SafeTxs whose threshold is only met by a mix of off-chain and
	// on-chain approvals — even though the Safe itself would accept them.
	if merged, err := safeContract.MergeOnChainApprovals(pending); err != nil {
		appUI.Warn("Couldn't fully merge on-chain approvals: %s (proceeding with %d off-chain sig(s))", err, len(pending.Sigs))
	} else if merged > 0 {
		appUI.Info("Merged %d on-chain approval(s) into signature set.", merged)
	}

	threshold, err := safeContract.Threshold()
	if err != nil {
		appUI.Error("Couldn't read safe threshold: %s", err)
		return
	}
	if uint64(len(pending.Sigs)) < threshold {
		appUI.Error(
			"Only %d signature(s) collected; threshold is %d. Ask remaining owners to approve first.",
			len(pending.Sigs), threshold,
		)
		return
	}

	if expected := pending.SafeTx.SafeTxHash(domainSep); expected != pending.SafeTxHash {
		appUI.Error(
			"safeTxHash from service (0x%s) doesn't match locally recomputed hash (0x%s); refusing to execute",
			ethcommon.Bytes2Hex(pending.SafeTxHash[:]),
			ethcommon.Bytes2Hex(expected[:]),
		)
		return
	}

	sigBlob, err := safe.EncodeSignatures(pending.Sigs)
	if err != nil {
		appUI.Error("Couldn't assemble signatures blob: %s", err)
		return
	}

	showSafeTxToConfirm(pending.SafeTx, pending.SafeTxHash, &tc)
	showSafeSigners("Signatures (sorted by owner asc)", pending.Sigs)

	txData, err := safeContract.Abi.Pack(
		"execTransaction",
		pending.SafeTx.To,
		pending.SafeTx.Value,
		pending.SafeTx.Data,
		uint8(pending.SafeTx.Operation),
		pending.SafeTx.SafeTxGas,
		pending.SafeTx.BaseGas,
		pending.SafeTx.GasPrice,
		pending.SafeTx.GasToken,
		pending.SafeTx.RefundReceiver,
		sigBlob,
	)
	if err != nil {
		appUI.Error("Couldn't pack execTransaction calldata: %s", err)
		return
	}

	gasLimit := config.GasLimit
	if gasLimit == 0 {
		gasLimit, err = tc.Reader.EstimateExactGas(tc.From, safeContract.Address, 0, tc.Value, txData)
		if err != nil {
			appUI.Error("Couldn't estimate gas limit for execTransaction: %s", err)
			return
		}
	}

	tx := jarviscommon.BuildExactTx(
		tc.TxType,
		tc.Nonce,
		safeContract.Address,
		tc.Value,
		gasLimit+config.ExtraGasLimit,
		tc.GasPrice+config.ExtraGasPrice,
		tc.TipGas+config.ExtraTipGas,
		txData,
		config.Network().GetChainID(),
	)

	customABIs := map[string]*abi.ABI{
		strings.ToLower(safeContract.Address): safeContract.Abi,
	}

	if broadcasted, err := cmdutil.SignAndBroadcast(
		appUI, tc.FromAcc, tx, customABIs,
		tc.Reader, tc.Analyzer, safeContract.Abi, tc.Broadcaster,
	); err != nil && !broadcasted {
		appUI.Error("Failed to proceed after signing the tx: %s. Aborted.", err)
	}
}

// nextSafeNonce returns the SafeTx nonce to use for a brand-new proposal.
// Honors --safe-nonce when set, else delegates to safe.NextNonce.
func nextSafeNonce(s *safe.SafeContract, c safe.SignatureCollector) (uint64, error) {
	if safeNonceOverride != 0 {
		return safeNonceOverride, nil
	}
	return safe.NextNonce(s, c)
}

// Safe-specific flag bindings are wired onto the unified `jarvis msig`
// commands in cmd/msig.go's init(). The cobra command vars in this file
// (initSafeCmd, approveSafeCmd, ...) are intentionally NOT registered as
// their own subcommand tree — the msig dispatcher invokes their .Run /
// .PersistentPreRunE closures directly after the unified preprocess
// detects a Gnosis Safe target. Keeping them as cobra structs (rather
// than free functions) is just a refactor convenience: it avoids
// touching the body of each Run closure.
