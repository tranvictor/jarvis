package cmd

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/accounts"
	jtypes "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/txanalyzer"
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

// txBuilderFile is the --tx-builder-file value: a path to a Safe{Wallet}
// Transaction Builder JSON export.
var txBuilderFile string

// txBuilderJSON is the --tx-builder-json value: the same export handed over as
// a literal JSON document, for pasting a batch straight onto the command line.
// Mutually exclusive with txBuilderFile.
var txBuilderJSON string

// multiSendAddressOverride is the --multisend-address value. When empty,
// jarvis probes the canonical MultiSendCallOnly deployments on-chain.
var multiSendAddressOverride string

// msigNewType is --type on `jarvis msig new`: "safe", "classic", or empty
// (prompt interactively; prefill mode defaults to classic for compatibility).
var msigNewType string

// Safe-deploy contract overrides for `jarvis msig new --type safe`. Empty
// means "probe the canonical Safe deployments on this chain".
var (
	msigNewFactory         string
	msigNewSingleton       string
	msigNewFallbackHandler string
)

// txBuilderBatch holds the parsed batch from --tx-builder-file or
// --tx-builder-json. It is populated by
// initMsigCmd's PersistentPreRunE (which has to parse the file before the
// preprocess pipeline runs, so the file's chainId can act as a network hint
// and its meta.createdFromSafeAddress can stand in for the positional Safe
// argument) and consumed by initSafeCmd's Run. nil means "not in batch mode".
var txBuilderBatch *safe.TxBuilderFile

// txBuilderExclusiveFlags are the interactive single-call flags that a batch
// makes meaningless. Silently ignoring them would let an operator believe a
// value/target they typed had been applied, so they're rejected outright.
var txBuilderExclusiveFlags = []string{
	"msig-to", "msig-value", "method-index", "no-func-call", "prefills",
}

// prepareTxBuilderBatch parses --tx-builder-file / --tx-builder-json (when
// given) and reconciles
// what the file says with what the operator typed. It returns the args slice
// the preprocess pipeline should see, which in batch mode may be synthesised
// from the file's meta.createdFromSafeAddress.
//
// It runs before any preprocess step, because the two facts it establishes —
// which chain and which Safe — are inputs to that pipeline. Every mismatch
// here is fatal by design: identical calldata replayed on the wrong chain or
// through the wrong Safe is precisely the accident this format can prevent.
func prepareTxBuilderBatch(cmd *cobra.Command, args []string) ([]string, error) {
	txBuilderBatch = nil

	path := strings.TrimSpace(txBuilderFile)
	inline := strings.TrimSpace(txBuilderJSON)

	switch {
	case path != "" && inline != "":
		return nil, fmt.Errorf(
			"--tx-builder-file and --tx-builder-json are mutually exclusive; pass one",
		)
	case path == "" && inline == "":
		if strings.TrimSpace(config.MsigTo) == "" {
			return nil, fmt.Errorf(
				"one of --msig-to, --tx-builder-file or --tx-builder-json is required",
			)
		}
		return args, nil
	}

	source := "--tx-builder-file"
	if inline != "" {
		source = "--tx-builder-json"
	}
	for _, name := range txBuilderExclusiveFlags {
		if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
			return nil, fmt.Errorf(
				"--%s can't be combined with %s; the batch supplies targets, values and parameters",
				name, source,
			)
		}
	}

	var (
		batch *safe.TxBuilderFile
		err   error
	)
	if inline != "" {
		batch, err = safe.ParseTxBuilderJSON(inline)
	} else {
		batch, err = safe.ReadTxBuilderFile(path)
	}
	if err != nil {
		return nil, err
	}

	chainID, err := batch.ChainIDUint()
	if err != nil {
		return nil, fmt.Errorf("tx builder batch: %w", err)
	}
	network, err := jarvisnetworks.GetNetworkByID(chainID)
	if err != nil {
		return nil, fmt.Errorf(
			"tx builder batch targets chain id %d which jarvis has no network for; "+
				"add it with 'jarvis network add' first", chainID,
		)
	}
	if cmd.Flags().Changed("network") {
		// Resolve the operator's --network the same way config.SetNetwork
		// would, so aliases ("bsc" vs "bsc-mainnet") compare correctly.
		chosen, err := jarvisnetworks.GetNetwork(config.NetworkString)
		if err != nil {
			return nil, err
		}
		if chosen.GetChainID() != chainID {
			return nil, fmt.Errorf(
				"tx builder batch targets chain id %d (%s) but --network=%s is chain id %d",
				chainID, network.GetName(), config.NetworkString, chosen.GetChainID(),
			)
		}
	} else {
		config.NetworkString = network.GetName()
	}

	if len(args) == 0 {
		fileSafe := batch.SafeAddress()
		if fileSafe == "" {
			return nil, fmt.Errorf(
				"no Safe address given and the tx builder batch has no usable " +
					"meta.createdFromSafeAddress; pass the Safe address explicitly",
			)
		}
		args = []string{fileSafe}
		appUI.Info("Safe (from %s): %s", source, fileSafe)
	} else if ethcommon.IsHexAddress(strings.TrimSpace(args[0])) {
		// Catch a plain-hex disagreement now, before jarvis spends RPC calls
		// probing an address the operator is going to have to retype anyway.
		// Aliases and Safe-app URLs can't be compared until the preprocess
		// pipeline has resolved them, so those fall through to the
		// authoritative check in initSafeCmd.Run.
		if err := assertTxBuilderSafeMatches(batch, strings.TrimSpace(args[0])); err != nil {
			return nil, err
		}
	}

	txBuilderBatch = batch
	return args, nil
}

// assertTxBuilderSafeMatches fails when the batch was exported from a
// different Safe than the one being proposed through. An empty
// meta.createdFromSafeAddress means the file doesn't claim a Safe, so there is
// nothing to contradict.
func assertTxBuilderSafeMatches(batch *safe.TxBuilderFile, safeAddress string) error {
	fileSafe := batch.SafeAddress()
	if fileSafe == "" {
		return nil
	}
	if !strings.EqualFold(fileSafe, ethcommon.HexToAddress(safeAddress).Hex()) {
		return fmt.Errorf(
			"tx builder batch was created from Safe %s but you asked to propose through %s",
			fileSafe, safeAddress,
		)
	}
	return nil
}

// buildTxBuilderSafeTx encodes the parsed batch into the four SafeTx fields
// that vary by input mode, plus the per-target ABIs the confirmation screen
// needs and a label describing the MultiSend contract in use.
//
// A single-entry batch is proposed as a plain CALL straight to its target
// rather than wrapped in MultiSend — that's what the Safe UI does too, and it
// avoids asking an operator to approve a DELEGATECALL for no reason.
//
// A nil data return means the failure was already reported to the UI.
func buildTxBuilderSafeTx(tc cmdutil.TxContext) (
	to string,
	value *big.Int,
	data []byte,
	op safe.Operation,
	abis map[string]*abi.ABI,
	label string,
) {
	network := config.Network()

	calls, err := txBuilderBatch.EncodeCalls(network)
	if err != nil {
		appUI.Error("Couldn't encode the tx builder batch: %s", err)
		return "", nil, nil, safe.OpCall, nil, ""
	}

	// Merged per address, not one entry per call: several entries hitting the
	// same contract must all stay decodable (see safe.MergeCallABIs).
	abis = safe.MergeCallABIs(calls)

	appUI.Section(fmt.Sprintf("Tx builder batch: %d transaction(s)", len(calls)))
	if name := strings.TrimSpace(txBuilderBatch.Meta.Name); name != "" {
		appUI.Info("Name        : %s", name)
	}
	if desc := strings.TrimSpace(txBuilderBatch.Meta.Description); desc != "" {
		appUI.Info("Description : %s", desc)
	}
	if v := strings.TrimSpace(txBuilderBatch.Meta.TxBuilderVersion); v != "" {
		appUI.Info("Builder ver : %s", v)
	}

	if len(calls) == 1 {
		c := calls[0]
		return c.To.Hex(), c.Value, c.Data, safe.OpCall, abis, ""
	}

	multiSend, label, err := safe.ResolveMultiSendCallOnly(
		tc.Safe, network, multiSendAddressOverride,
	)
	if err != nil {
		appUI.Error("%s", err)
		return "", nil, nil, safe.OpCall, nil, ""
	}

	msCalls := make([]jarviscommon.MultiSendCall, 0, len(calls))
	for _, c := range calls {
		msCalls = append(msCalls, c.MultiSendCall)
	}
	packed, err := jarviscommon.PackMultiSend(msCalls)
	if err != nil {
		appUI.Error("Couldn't pack the MultiSend batch: %s", err)
		return "", nil, nil, safe.OpCall, nil, ""
	}

	// Value stays 0: MultiSend runs as a delegatecall in the Safe's own
	// context, so each sub-call spends the Safe's balance directly. Passing
	// the sum here would try to send the Safe its own ether.
	return multiSend.Hex(), big.NewInt(0), packed, safe.OpDelegateCall,
		abis, fmt.Sprintf("%s (%s)", multiSend.Hex(), label)
}

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

// safeBatchResult is the per-ref outcome of `jarvis msig bapprove`.
type safeBatchResult struct {
	ref         string
	network     string
	networkObj  jarvisnetworks.Network
	safeAddress string
	safeTxHash  string
	confirmType string // "approve" / "approve+execute" / "" when nothing happened
	execTxHash  string
	status      string // "approved", "executed", "skipped", "failed"
	reason      string
}

// safeRefInput pairs the canonical SafeAppRef with the original token
// the user wrote so error messages can quote what they typed.
type safeRefInput struct {
	original string
	ref      *safe.SafeAppRef
}

// scanForSafeRefs splits raw on whitespace/commas and tries to parse each
// fragment as a Safe reference. Tokens that don't parse are silently
// dropped — they're typically commentary or shell artifacts. Tokens that
// parse but lack a tx hash are also dropped because we can't approve them.
func scanForSafeRefs(raw string) []safeRefInput {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t' || r == '|'
	})
	var out []safeRefInput
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		// Support `<chain>:<safe>:<hash>` — three colon-separated parts —
		// by rewriting to the EIP-3770 + multisig token form ParseSafeAppURL
		// already understands. Anything else falls through to the parser.
		if extra := chainSafeHashRe.FindStringSubmatch(f); extra != nil {
			ref := &safe.SafeAppRef{
				ChainShortName: strings.ToLower(extra[1]),
				SafeAddress:    ethcommon.HexToAddress(extra[2]),
			}
			ref.ChainID = safe.ShortNameChainID(ref.ChainShortName)
			copy(ref.SafeTxHash[:], ethcommon.FromHex(extra[3]))
			out = append(out, safeRefInput{original: f, ref: ref})
			continue
		}
		if ref, ok := safe.ParseSafeAppURL(f); ok && ref.HasTxHash() {
			out = append(out, safeRefInput{original: f, ref: ref})
		}
	}
	return out
}

// chainSafeHashRe matches `<chain>:<safe>:<hash>` shorthand for
// "this safe on this chain has this pending tx" — useful for clipboard
// paste from spreadsheets / CSVs. Does NOT match Safe URLs (those have `?`).
var chainSafeHashRe = regexp.MustCompile(
	`(?i)^([a-z]{2,10}[0-9]{0,3}):(0x[0-9a-f]{40}):(0x[0-9a-f]{64})$`,
)

// approveSafeRefResult is what approveSafeRef returns for the batch
// summary; it's a flat shape with no business logic of its own.
type approveSafeRefResult struct {
	network     string
	networkObj  jarvisnetworks.Network
	safeAddress string
	safeTxHash  string
	confirmType string
	execTxHash  string
	status      string
	reason      string
}

// approveSafeRef performs the same logical steps as `jarvis msig approve`
// for one ref: resolve the network + safe, find a local owner wallet,
// sign the EIP-712 hash, submit to the Tx Service, and (when this approval
// brings the count past threshold and --no-execute is not set) chain into
// runSafeExecute. Errors are returned as `failed`/`skipped` results rather
// than propagated, so the batch keeps going.
func approveSafeRef(in safeRefInput) approveSafeRefResult {
	ref := in.ref
	res := approveSafeRefResult{
		safeAddress: ref.SafeAddress.Hex(),
		safeTxHash:  ref.SafeTxHashHex(),
	}
	if ref.ChainID == 0 {
		res.status = "skipped"
		res.reason = fmt.Sprintf("no chain hint in %q (use a URL or <chain>:<safe>:<hash>)", in.original)
		appUI.Warn("%s", res.reason)
		return res
	}
	network, err := jarvisnetworks.GetNetworkByID(ref.ChainID)
	if err != nil {
		res.status = "skipped"
		res.reason = fmt.Sprintf("unsupported chain id %d", ref.ChainID)
		appUI.Warn("%s", res.reason)
		return res
	}
	res.network = network.GetName()
	res.networkObj = network
	appUI.Info("Network: %s, Safe: %s", network.GetName(), ref.SafeAddress.Hex())

	safeContract, err := safe.NewSafeContract(ref.SafeAddress.Hex(), network)
	if err != nil {
		res.status = "failed"
		res.reason = fmt.Sprintf("init safe reader: %s", err)
		appUI.Error("%s", res.reason)
		return res
	}
	owners, err := safeContract.Owners()
	if err != nil {
		res.status = "failed"
		res.reason = fmt.Sprintf("read safe owners: %s", err)
		appUI.Error("%s", res.reason)
		return res
	}

	var fromAcc jtypes.AccDesc
	if config.From == "" {
		acc, _, err := cmdutil.PickLocalOwner(owners, config.From, cmdutil.OwnerRequireUnique)
		if errors.Is(err, cmdutil.ErrNoLocalOwner) {
			res.status = "skipped"
			res.reason = "no local wallet is an owner of this safe"
			appUI.Warn("%s", res.reason)
			return res
		}
		if errors.Is(err, cmdutil.ErrMultipleLocalOwners) {
			res.status = "skipped"
			res.reason = "multiple local owner wallets; pass --from to disambiguate"
			appUI.Warn("%s", res.reason)
			return res
		}
		if err != nil {
			res.status = "failed"
			res.reason = err.Error()
			appUI.Error("%s", res.reason)
			return res
		}
		fromAcc = acc
	} else {
		acc, _, err := cmdutil.ResolveAccount(cmdutil.DefaultABIResolver{}, config.From)
		if err != nil {
			res.status = "failed"
			res.reason = fmt.Sprintf("--from %q: %s", config.From, err)
			appUI.Error("%s", res.reason)
			return res
		}
		if !cmdutil.IsAmongOwners(owners, acc.Address) {
			res.status = "skipped"
			res.reason = fmt.Sprintf("--from %s is not an owner of this safe", acc.Address)
			appUI.Warn("%s", res.reason)
			return res
		}
		fromAcc = acc
	}

	collector, err := safe.NewTxServiceCollector(network.GetChainID())
	if err != nil {
		res.status = "failed"
		res.reason = fmt.Sprintf("init safe tx service: %s", err)
		appUI.Error("%s", res.reason)
		return res
	}
	pending, err := collector.Get(ref.SafeTxHash)
	if err != nil {
		res.status = "failed"
		res.reason = fmt.Sprintf("fetch pending tx: %s", err)
		appUI.Error("%s", res.reason)
		return res
	}
	if pending.IsExecuted {
		res.status = "skipped"
		res.reason = "already executed"
		appUI.Warn("%s", res.reason)
		return res
	}

	domainSep, err := safeContract.DomainSeparator()
	if err != nil {
		res.status = "failed"
		res.reason = fmt.Sprintf("read domainSeparator: %s", err)
		appUI.Error("%s", res.reason)
		return res
	}
	if _, err := verifyPendingSafeTxHash(pending, domainSep); errors.Is(err, errPendingHashMismatch) {
		res.status = "failed"
		res.reason = "service safeTxHash doesn't match locally recomputed hash"
		appUI.Error("%s", res.reason)
		return res
	}

	// Merge on-chain approvals so the self-signed check and display
	// reflect the true state of the Safe, not just the Transaction
	// Service's view. Failures are tolerated — the worst outcome is that
	// we ask an owner to re-sign something they already on-chain
	// approved, and the Safe would reject that at execution time.
	if _, err := safeContract.MergeOnChainApprovals(pending); err != nil {
		appUI.Warn("couldn't merge on-chain approvals: %s", err)
	}

	me := ethcommon.HexToAddress(fromAcc.Address)
	if onChain, found := ownerAlreadySigned(pending, me); found {
		res.status = "skipped"
		if onChain {
			res.reason = fmt.Sprintf("%s already approved on-chain", me.Hex())
		} else {
			res.reason = fmt.Sprintf("%s already signed off-chain", me.Hex())
		}
		appUI.Warn("%s", res.reason)
		return res
	}

	// Build a TxContext rich enough for showSafeTxToConfirm and (for the
	// optional auto-execute step) for runSafeExecute. We deliberately do
	// the gas/nonce/tx-type lookups here rather than relying on a global
	// preprocess because this command runs across many networks.
	tc, err := buildTxContextForBatch(network, fromAcc, safeContract, collector)
	if err != nil {
		res.status = "failed"
		res.reason = err.Error()
		appUI.Error("%s", res.reason)
		return res
	}

	showSafeTxToConfirm(pending.SafeTx, pending.SafeTxHash, &tc)
	showSafeSigners("Existing signatures", pending.Sigs)

	if safeApproveOnChain {
		// On-chain batch approval: broadcast approveHash and let
		// runSafeApproveOnChain handle the (possibly auto-executing)
		// follow-up. We don't attempt to distinguish between approved
		// and approved+executed outcomes in the summary here because
		// runSafeApproveOnChain doesn't report that back — the user
		// can still see what happened in the stream above.
		if !config.YesToAllPrompt && !appUI.Confirm("Broadcast approveHash on-chain?", true) {
			res.status = "skipped"
			res.reason = "user aborted"
			return res
		}
		runSafeApproveOnChain(tc, safeContract, pending, domainSep, me)
		res.status = "approved"
		res.confirmType = "approve-onchain"
		return res
	}

	if !config.YesToAllPrompt && !appUI.Confirm("Sign and submit your approval?", true) {
		res.status = "skipped"
		res.reason = "user aborted"
		return res
	}

	appUI.Info("Unlock %s and sign the EIP-712 safeTxHash now...", fromAcc.Address)
	sig, err := signPendingSafeTx(fromAcc, pending, domainSep)
	if errors.Is(err, errSignSafeHash) {
		res.status = "failed"
		res.reason = fmt.Sprintf("sign safeTxHash: %s", signCause(err))
		appUI.Error("%s", res.reason)
		return res
	}
	if err != nil {
		res.status = "failed"
		res.reason = fmt.Sprintf("unlock wallet: %s", err)
		appUI.Error("%s", res.reason)
		return res
	}

	if err := persistApproval(tc, safeContract.Address, pending, me, sig, "", network.GetChainID()); err != nil {
		res.status = "failed"
		res.reason = fmt.Sprintf("submit confirmation: %s", err)
		appUI.Error("%s", res.reason)
		return res
	}
	res.status = "approved"
	res.confirmType = "approve"
	appUI.Success("Confirmation submitted.")

	threshold, err := safeContract.Threshold()
	if err != nil {
		appUI.Warn("Couldn't read threshold post-approval: %s", err)
		return res
	}
	totalSigs := len(pending.Sigs) + 1
	if uint64(totalSigs) < threshold || safeNoExecute {
		return res
	}

	if !config.YesToAllPrompt && !appUI.Confirm("Threshold met — broadcast execTransaction now?", true) {
		appUI.Info("Skipping execution. Run later with `jarvis msig execute ...`.")
		return res
	}

	runSafeExecute(tc, safeContract, pendingWithNewSig(pending, me, sig), domainSep)
	res.confirmType = "approve+execute"
	res.status = "executed"
	return res
}

// buildTxContextForBatch fills in a TxContext for the cross-network batch
// case where each iteration has its own network/wallet. Mirrors the
// gas/nonce/tx-type resolution CommonSafeTxPreprocess does.
func buildTxContextForBatch(
	network jarvisnetworks.Network,
	fromAcc jtypes.AccDesc,
	safeContract *safe.SafeContract,
	collector safe.SignatureCollector,
) (cmdutil.TxContext, error) {
	r, err := util.EthReader(network)
	if err != nil {
		return cmdutil.TxContext{}, fmt.Errorf("connect to blockchain: %w", err)
	}
	bc, err := util.EthBroadcaster(network)
	if err != nil {
		return cmdutil.TxContext{}, fmt.Errorf("connect to broadcaster: %w", err)
	}
	tc := cmdutil.TxContext{
		Reader:      r,
		Analyzer:    txanalyzer.NewGenericAnalyzer(r, network),
		Resolver:    cmdutil.DefaultABIResolver{},
		Broadcaster: bc,
		Safe:        safeContract,
		Collector:   collector,
		FromAcc:     fromAcc,
		From:        fromAcc.Address,
	}
	if err := cmdutil.FillSigningTxParams(appUI, &tc, network); err != nil {
		return cmdutil.TxContext{}, err
	}
	return tc, nil
}

// printSafeBatchSummary renders a per-ref outcome list followed by a
// totals line, mirroring printBatchSummary for classic msig.
func printSafeBatchSummary(results []safeBatchResult) {
	appUI.Section("Batch Approve Summary")
	approved, executed, skipped, failed := 0, 0, 0, 0
	for i, r := range results {
		safeLabel := ""
		if r.safeAddress != "" {
			safeLabel = fmt.Sprintf(" safe %s", r.safeAddress)
		}
		switch r.status {
		case "approved":
			approved++
			appUI.Success("  %d. [%s]%s — approved", i+1, r.network, safeLabel)
		case "executed":
			executed++
			appUI.Success("  %d. [%s]%s — approved + executed", i+1, r.network, safeLabel)
		case "skipped":
			skipped++
			appUI.Warn("  %d. [%s]%s — skipped: %s", i+1, r.network, safeLabel, r.reason)
		case "failed":
			failed++
			appUI.Error("  %d. [%s]%s — failed: %s", i+1, r.network, safeLabel, r.reason)
		}
		if r.safeTxHash != "" {
			appUI.Info("       safeTxHash %s", r.safeTxHash)
		}
		if r.execTxHash != "" {
			appUI.Info("       exec tx    %s", r.execTxHash)
		}
	}
	parts := []string{}
	if approved > 0 {
		parts = append(parts, fmt.Sprintf("%d approved", approved))
	}
	if executed > 0 {
		parts = append(parts, fmt.Sprintf("%d executed", executed))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	appUI.Info("")
	appUI.Info("Total: %d transactions (%s)", len(results), strings.Join(parts, ", "))
}

// jsonSafeBatchResult and jsonSafeBatchSummary mirror the classic-msig
// JSON shapes so consumers (CI scripts, dashboards) can treat the two
// commands interchangeably when their output file is provided.
type jsonSafeBatchResult struct {
	Ref         string `json:"ref"`
	Network     string `json:"network,omitempty"`
	Safe        string `json:"safe,omitempty"`
	SafeTxHash  string `json:"safe_tx_hash,omitempty"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
	ConfirmType string `json:"confirm_type,omitempty"`
	ExecTxHash  string `json:"exec_tx_hash,omitempty"`
}

type jsonSafeBatchSummary struct {
	Total     int                   `json:"total"`
	Approved  int                   `json:"approved"`
	Executed  int                   `json:"executed"`
	Skipped   int                   `json:"skipped"`
	Failed    int                   `json:"failed"`
	Generated string                `json:"generated_at"`
	Results   []jsonSafeBatchResult `json:"results"`
}

func buildSafeBatchSummary(results []safeBatchResult) jsonSafeBatchSummary {
	out := jsonSafeBatchSummary{
		Total:     len(results),
		Generated: time.Now().UTC().Format(time.RFC3339),
		Results:   make([]jsonSafeBatchResult, 0, len(results)),
	}
	for _, r := range results {
		out.Results = append(out.Results, jsonSafeBatchResult{
			Ref:         r.ref,
			Network:     r.network,
			Safe:        r.safeAddress,
			SafeTxHash:  r.safeTxHash,
			Status:      r.status,
			Reason:      r.reason,
			ConfirmType: r.confirmType,
			ExecTxHash:  r.execTxHash,
		})
		switch r.status {
		case "approved":
			out.Approved++
		case "executed":
			out.Executed++
		case "skipped":
			out.Skipped++
		case "failed":
			out.Failed++
		}
	}
	return out
}

func writeSafeBatchSummaryJSON(path string, results []safeBatchResult) {
	writeJSONSummary(path, buildSafeBatchSummary(results))
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

// showSafeInfo prints owner list / threshold / version / nonce so the user
// has confidence about which Safe they're operating on. Failures here are
// non-fatal — they just degrade the displayed information.
func showSafeInfo(s *safe.SafeContract) {
	appUI.Info("Safe address : %s", s.Address)
	if v, err := s.Version(); err == nil {
		appUI.Info("Safe version : %s", v)
	}
	if n, err := s.Nonce(); err == nil {
		appUI.Info("Safe nonce   : %d (next on-chain executable)", n)
	}
	if t, err := s.Threshold(); err == nil {
		appUI.Info("Threshold    : %d", t)
	}
	if owners, err := s.Owners(); err == nil {
		appUI.Info("Owners (%d):", len(owners))
		for i, o := range owners {
			jarvisAddr := util.GetJarvisAddress(o, config.Network())
			appUI.Info("  %d. %s", i+1, appUI.Style(util.StyledAddress(jarvisAddr)))
		}
	}
}

// showSafeTxToConfirm displays the parameters of a SafeTx in a way that
// matches Safe wallet UIs (so users can sanity-check side-by-side) AND
// decodes the calldata into a human-readable function call using jarvis's
// standard analyzer pipeline — exactly the way `jarvis msig` shows pending
// classic-multisig transactions. Pass tc so we can reach the network reader,
// analyzer, and ABI resolver; pass nil to fall back to a raw-hex display.
func showSafeTxToConfirm(stx *safe.SafeTx, hash [32]byte, tc *cmdutil.TxContext) {
	showSafeTxToConfirmWithABIs(stx, hash, tc, nil)
}

// showSafeTxToConfirmWithABIs is showSafeTxToConfirm plus a set of extra ABIs
// keyed by lowercased address. Batch proposals need it: the calls inside a
// MultiSend payload frequently target contracts the block explorer can't give
// an ABI for, and jarvis already knows their signatures because it just
// encoded them from the tx builder batch.
func showSafeTxToConfirmWithABIs(
	stx *safe.SafeTx,
	hash [32]byte,
	tc *cmdutil.TxContext,
	extraABIs map[string]*abi.ABI,
) {
	appUI.Section("Safe transaction details")

	// util.GetJarvisAddress runs through util.NewEnrichedResolver, which
	// transparently fetches verified contract names (and follows
	// proxies) from the block explorer on first miss — no manual
	// PrefetchContractName plumbing required here or in the analyzer
	// pipeline below.
	toJarvis := util.GetJarvisAddress(stx.To.Hex(), config.Network())
	appUI.Critical("To             : %s", appUI.Style(util.StyledAddress(toJarvis)))

	if stx.Value != nil && stx.Value.Sign() > 0 {
		appUI.Critical("Value          : %f %s (%s wei)",
			jarviscommon.BigToFloat(stx.Value, config.Network().GetNativeTokenDecimal()),
			config.Network().GetNativeTokenSymbol(),
			stx.Value.String(),
		)
	} else {
		appUI.Critical("Value          : 0")
	}
	appUI.Critical("Operation      : %s", operationLabel(stx.Operation))
	if stx.Operation == safe.OpDelegateCall && jarviscommon.IsMultiSendCallData(stx.Data) {
		// The DANGEROUS label above stays: a delegatecall really does run
		// foreign code in the Safe's context. This adds the missing why, so a
		// reviewing owner can tell a routine batch from an actual red flag.
		appUI.Critical("                 ^ this is a MultiSend batch; every call it makes is listed below")
	}
	appUI.Critical("Nonce (Safe)   : %s", stx.Nonce.String())
	appUI.Critical("safeTxGas      : %s", stx.SafeTxGas.String())
	appUI.Critical("baseGas        : %s", stx.BaseGas.String())
	appUI.Critical("gasPrice       : %s", stx.GasPrice.String())
	appUI.Critical("gasToken       : %s", stx.GasToken.Hex())
	appUI.Critical("refundReceiver : %s", stx.RefundReceiver.Hex())
	appUI.Critical("safeTxHash     : 0x%s", ethcommon.Bytes2Hex(hash[:]))

	if len(stx.Data) == 0 {
		appUI.Critical("Data           : (empty)")
		return
	}

	// Decoded calldata block. We mirror cmd/util.AnalyzeAndShowMsigTxInfo:
	// fetch the destination ABI through the resolver (honoring --custom-abi
	// and --erc20), then hand off to util.AnalyzeMethodCallAndPrint which
	// prints the function name + decoded params with token-aware formatting.
	if tc == nil || tc.Resolver == nil || tc.Analyzer == nil {
		appUI.Critical("Data (%d bytes): 0x%s", len(stx.Data), ethcommon.Bytes2Hex(stx.Data))
		return
	}
	customABIs := map[string]*abi.ABI{}
	for addr, a := range extraABIs {
		customABIs[strings.ToLower(addr)] = a
	}

	destAbi, err := tc.Resolver.ConfigToABI(
		stx.To.Hex(), config.ForceERC20ABI, config.CustomABI, config.Network(),
	)
	if err != nil {
		// MultiSend / MultiSendCallOnly is unverified on many explorers, so a
		// failure here is expected for batches and must not abort the decode:
		// the analyzer has a built-in multiSend ABI to fall back on.
		appUI.Warn("Couldn't resolve ABI of destination %s: %s", stx.To.Hex(), err)
		if len(customABIs) == 0 && !jarviscommon.IsMultiSendCallData(stx.Data) {
			appUI.Critical("Data (%d bytes): 0x%s", len(stx.Data), ethcommon.Bytes2Hex(stx.Data))
			return
		}
	} else if _, taken := customABIs[strings.ToLower(stx.To.Hex())]; !taken {
		customABIs[strings.ToLower(stx.To.Hex())] = destAbi
	}

	util.AnalyzeMethodCallAndPrint(
		appUI,
		tc.Analyzer,
		stx.Value,
		stx.To.Hex(),
		stx.Data,
		customABIs,
		config.Network(),
	)
}

// showSafeSigners renders the list of owners that have already signed,
// resolving each address through the jarvis address book so names show up
// the same way `jarvis msig` displays confirmation lists. Entries produced
// by OnChainApprovalSig (v=0) are tagged "[on-chain]" so users can tell at a
// glance which owners approved via approveHash rather than off-chain signing.
func showSafeSigners(label string, sigs []safe.OwnerSig) {
	if len(sigs) == 0 {
		appUI.Info("%s: (none yet)", label)
		return
	}
	appUI.Info("%s (%d):", label, len(sigs))
	for i, s := range sigs {
		jarvisAddr := util.GetJarvisAddress(s.Owner.Hex(), config.Network())
		tag := "[off-chain]"
		if safe.IsOnChainApproval(s.Sig) {
			tag = "[on-chain] "
		}
		appUI.Info("  %d. %s %s", i+1, tag, appUI.Style(util.StyledAddress(jarvisAddr)))
	}
}

func operationLabel(op safe.Operation) string {
	switch op {
	case safe.OpCall:
		return "CALL (0)"
	case safe.OpDelegateCall:
		return "DELEGATECALL (1) — DANGEROUS"
	default:
		return fmt.Sprintf("UNKNOWN (%d)", op)
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

var (
	errPendingNoCollector  = errors.New("safe transaction service is not available")
	errPendingNeedHash     = errors.New("on-chain approve without a service requires an explicit safeTxHash")
	errPendingHashMismatch = errors.New("safeTxHash mismatch")
	errSignSafeHash        = errors.New("sign safeTxHash")
)

// verifyPendingSafeTxHash recomputes the EIP-712 hash when a SafeTx body
// is present. Hash-only --approve-onchain stubs skip the check (nil error).
func verifyPendingSafeTxHash(pending *safe.PendingTx, domainSep [32]byte) ([32]byte, error) {
	var expected [32]byte
	if pending == nil || pending.SafeTx == nil {
		return expected, nil
	}
	expected = pending.SafeTx.SafeTxHash(domainSep)
	if expected != pending.SafeTxHash {
		return expected, errPendingHashMismatch
	}
	return expected, nil
}

func ownerAlreadySigned(pending *safe.PendingTx, me ethcommon.Address) (onChain bool, found bool) {
	if pending == nil {
		return false, false
	}
	for _, s := range pending.Sigs {
		if s.Owner == me {
			return safe.IsOnChainApproval(s.Sig), true
		}
	}
	return false, false
}

func signSafeTx(fromAcc jtypes.AccDesc, stx *safe.SafeTx, domainSep [32]byte) ([]byte, error) {
	account, err := accounts.UnlockAccount(fromAcc)
	if err != nil {
		return nil, err
	}
	sig, err := account.SignSafeHash(domainSep, stx.StructHash())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errSignSafeHash, err)
	}
	return sig, nil
}

func signPendingSafeTx(fromAcc jtypes.AccDesc, pending *safe.PendingTx, domainSep [32]byte) ([]byte, error) {
	return signSafeTx(fromAcc, pending.SafeTx, domainSep)
}

func signCause(err error) string {
	prefix := errSignSafeHash.Error() + ": "
	if strings.HasPrefix(err.Error(), prefix) {
		return strings.TrimPrefix(err.Error(), prefix)
	}
	return err.Error()
}

func persistApproval(
	tc cmdutil.TxContext,
	safeAddr string,
	pending *safe.PendingTx,
	me ethcommon.Address,
	sig []byte,
	file string,
	chainID uint64,
) error {
	if file != "" {
		updated := append(append([]safe.OwnerSig{}, pending.Sigs...), safe.OwnerSig{Owner: me, Sig: sig})
		return safe.WriteTxFile(file, safeAddr, chainID, pending.SafeTx, pending.SafeTxHash, updated)
	}
	return tc.Collector.Confirm(pending.SafeTxHash, me, sig)
}

func pendingWithNewSig(pending *safe.PendingTx, me ethcommon.Address, sig []byte) *safe.PendingTx {
	augmented := *pending
	augmented.Sigs = append(append([]safe.OwnerSig{}, pending.Sigs...), safe.OwnerSig{Owner: me, Sig: sig})
	return &augmented
}

// loadPendingTx is the shared pending-tx source selector for approve /
// execute / info: local file, Safe Transaction Service, or (approve only)
// a hash-only stub when --approve-onchain is used without a service.
func loadPendingTx(tc cmdutil.TxContext, args []string, file string, allowOnChainHash bool) (*safe.PendingTx, error) {
	if file != "" {
		_, pending, err := loadPendingFromTxFile(tc, file)
		return pending, err
	}
	if tc.Collector == nil {
		if allowOnChainHash {
			return pendingFromOnChainHash(tc, args)
		}
		return nil, errPendingNoCollector
	}
	identifier, err := pickPendingTxIdentifier(tc, args)
	if err != nil {
		return nil, err
	}
	return resolvePendingTx(tc, identifier)
}

// pendingFromOnChainHash builds a hash-only PendingTx when there is no
// Transaction Service. The caller must have passed a Safe-app URL with a
// hash or a 32-byte 0x… positional arg.
func pendingFromOnChainHash(tc cmdutil.TxContext, args []string) (*safe.PendingTx, error) {
	if tc.SafeAppRef != nil && tc.SafeAppRef.HasTxHash() {
		var hash [32]byte
		copy(hash[:], tc.SafeAppRef.SafeTxHash[:])
		return &safe.PendingTx{
			Safe:       ethcommon.HexToAddress(tc.Safe.Address),
			SafeTxHash: hash,
		}, nil
	}
	if len(args) >= 2 {
		if hash, ok := parseSafeTxHashArg(args[1]); ok {
			return &safe.PendingTx{
				Safe:       ethcommon.HexToAddress(tc.Safe.Address),
				SafeTxHash: hash,
			}, nil
		}
	}
	return nil, errPendingNeedHash
}

func parseSafeTxHashArg(s string) ([32]byte, bool) {
	s = strings.TrimSpace(s)
	var hash [32]byte
	if !strings.HasPrefix(strings.ToLower(s), "0x") || len(s) != 66 {
		return hash, false
	}
	copy(hash[:], ethcommon.FromHex(s))
	return hash, true
}

// loadPendingFromTxFile reads safeTxFile, cross-checks that it matches
// the Safe and chain the user is currently targeting, and returns a
// PendingTx in the same shape the Safe Transaction Service would. It is
// the --safe-tx-file counterpart to pickPendingTxIdentifier +
// resolvePendingTx, used when no service is available or when the user
// deliberately chose file-based signing. Returns the file handle too so
// callers that need to append a signature (approve, off-chain) can write
// back to it without re-reading.
func loadPendingFromTxFile(tc cmdutil.TxContext, path string) (*safe.TxFile, *safe.PendingTx, error) {
	tf, err := safe.ReadTxFile(path)
	if err != nil {
		return nil, nil, err
	}
	if tf.ChainID != config.Network().GetChainID() {
		return nil, nil, fmt.Errorf(
			"tx file %s was built for chain %d but current --network is chain %d",
			path, tf.ChainID, config.Network().GetChainID(),
		)
	}
	if !strings.EqualFold(tf.Safe, tc.Safe.Address) {
		return nil, nil, fmt.Errorf(
			"tx file %s is for safe %s, but this command targets %s",
			path, tf.Safe, tc.Safe.Address,
		)
	}
	pending, err := tf.ToPending()
	if err != nil {
		return nil, nil, fmt.Errorf("decode tx file: %w", err)
	}
	return tf, pending, nil
}

// pickPendingTxIdentifier returns the safeTxHash / nonce string that
// resolvePendingTx should look up. Precedence:
//
//  1. A safeTxHash carried in a Safe-app URL (tc.SafeAppRef.SafeTxHash).
//  2. The second positional argument, if present.
//  3. Auto-pick when the Safe Transaction Service has exactly one pending
//     tx for this Safe (mirrors `jarvis msig` UX).
//
// When several pending txs exist, the function prints a numbered menu and
// errors with an actionable message instead of guessing.
func pickPendingTxIdentifier(tc cmdutil.TxContext, args []string) (string, error) {
	if tc.SafeAppRef != nil && tc.SafeAppRef.HasTxHash() {
		return tc.SafeAppRef.SafeTxHashHex(), nil
	}
	if len(args) >= 2 {
		return args[1], nil
	}
	pending, err := tc.Collector.ListPending(ethcommon.HexToAddress(tc.Safe.Address))
	if err != nil {
		return "", fmt.Errorf(
			"no pending tx identifier given and listing the Safe Transaction Service queue failed: %w",
			err,
		)
	}
	switch len(pending) {
	case 0:
		return "", fmt.Errorf(
			"no pending Safe transactions found for %s. Initiate one with `jarvis msig init`, or pass a safeTxHash / nonce explicitly.",
			tc.Safe.Address,
		)
	case 1:
		p := pending[0]
		appUI.Info(
			"Auto-selected the only pending Safe tx (nonce %s, safeTxHash 0x%s).",
			p.SafeTx.Nonce.String(), ethcommon.Bytes2Hex(p.SafeTxHash[:]),
		)
		return "0x" + ethcommon.Bytes2Hex(p.SafeTxHash[:]), nil
	default:
		appUI.Warn("There are %d pending Safe transactions for %s:", len(pending), tc.Safe.Address)
		for i, p := range pending {
			appUI.Info(
				"  %d. nonce %s  to %s  safeTxHash 0x%s  sigs %d",
				i+1, p.SafeTx.Nonce.String(), p.SafeTx.To.Hex(),
				ethcommon.Bytes2Hex(p.SafeTxHash[:]), len(p.Sigs),
			)
		}
		return "", fmt.Errorf(
			"please specify which one by safeTxHash, nonce, or full Safe-app URL",
		)
	}
}

// resolvePendingTx looks up a pending tx by either safeTxHash (32-byte hex)
// or SafeTx nonce (decimal integer).
func resolvePendingTx(tc cmdutil.TxContext, identifier string) (*safe.PendingTx, error) {
	identifier = strings.TrimSpace(identifier)
	if hash, ok := parseSafeTxHashArg(identifier); ok {
		pt, err := tc.Collector.Get(hash)
		if err != nil {
			return nil, fmt.Errorf("fetching tx 0x%s from service: %w", ethcommon.Bytes2Hex(hash[:]), err)
		}
		return pt, nil
	}
	nonce, err := util.ParamToBigInt(identifier)
	if err != nil {
		return nil, fmt.Errorf("can't interpret %q as a safeTxHash or a nonce", identifier)
	}
	if !nonce.IsUint64() {
		return nil, fmt.Errorf("nonce %s is out of range", identifier)
	}
	pt, err := tc.Collector.FindByNonce(
		ethcommon.HexToAddress(tc.Safe.Address), nonce.Uint64(),
	)
	if err != nil {
		return nil, fmt.Errorf("looking up nonce %s in service queue: %w", identifier, err)
	}
	if pt == nil {
		return nil, fmt.Errorf("no pending Safe transaction at nonce %s", identifier)
	}
	return pt, nil
}

// Safe-specific flag bindings are wired onto the unified `jarvis msig`
// commands in cmd/msig.go's init(). The cobra command vars in this file
// (initSafeCmd, approveSafeCmd, ...) are intentionally NOT registered as
// their own subcommand tree — the msig dispatcher invokes their .Run /
// .PersistentPreRunE closures directly after the unified preprocess
// detects a Gnosis Safe target. Keeping them as cobra structs (rather
// than free functions) is just a refactor convenience: it avoids
// touching the body of each Run closure.
