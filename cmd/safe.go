package cmd

import (
	"encoding/json"
	"fmt"
	"os"
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
// `jarvis safe approve`. By default, when an approval brings the signature
// count to or above the Safe's threshold, jarvis chains an execute in the
// same invocation so the last signer doesn't have to run a second command.
// Setting --no-execute keeps the legacy "approve only" behavior.
var safeNoExecute bool

var safeCmd = &cobra.Command{
	Use:   "safe",
	Short: "Gnosis Safe (v1.x / v1.3+ / v1.4) multisig operations",
	Long: `jarvis safe lets you propose, approve and execute Gnosis Safe
multisig transactions using the Safe Transaction Service for off-chain
signature collection. The UX deliberately mirrors 'jarvis msig' (which
targets Gnosis Classic), so the same -j / -V / -M / -I / -S flags work.

v1 supports init / approve / execute. Hardware wallets (Ledger, Trezor)
are supported alongside private-key and keystore wallets.`,
}

var initSafeCmd = &cobra.Command{
	Use:   "init",
	Short: "Propose a new Safe transaction (off-chain via Safe Transaction Service)",
	Long: `Build a SafeTx targeting --msig-to with the call data interactively
constructed from the target's ABI, sign the EIP-712 safeTxHash with --from
(or the only owner you have a wallet for), and submit the proposal to the
Safe Transaction Service. Other owners can later approve via 'jarvis safe
approve' and any owner can finalise via 'jarvis safe execute'.`,
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

		valueWei := jarviscommon.FloatToBigInt(
			config.MsigValue, config.Network().GetNativeTokenDecimal(),
		)
		stx := safe.NewSafeTx(
			ethcommon.HexToAddress(config.MsigTo),
			valueWei,
			callData,
			safe.OpCall,
			safeNonce,
		)
		hash := stx.SafeTxHash(domainSep)

		showSafeTxToConfirm(stx, hash, &tc)
		if !config.YesToAllPrompt && !appUI.Confirm("Sign and submit this Safe transaction?", true) {
			appUI.Warn("Aborted by user.")
			return
		}

		appUI.Info("Unlock your wallet and sign the EIP-712 safeTxHash now...")
		account, err := accounts.UnlockAccount(tc.FromAcc)
		if err != nil {
			appUI.Error("Couldn't unlock wallet: %s", err)
			return
		}

		structHash := stx.StructHash()
		sig, err := account.SignSafeHash(domainSep, structHash)
		if err != nil {
			appUI.Error("Couldn't sign safeTxHash: %s", err)
			return
		}

		if err := tc.Collector.Propose(
			ethcommon.HexToAddress(safeContract.Address),
			stx, hash,
			ethcommon.HexToAddress(tc.From),
			sig,
		); err != nil {
			appUI.Error("Submitting proposal to Safe Transaction Service failed: %s", err)
			return
		}

		appUI.Success("Proposal submitted.")
		appUI.Info("safeTxHash: 0x%s", ethcommon.Bytes2Hex(hash[:]))
		appUI.Info("Other owners can approve with:")
		appUI.Info("  jarvis safe approve %s 0x%s", safeContract.Address, ethcommon.Bytes2Hex(hash[:]))
		appUI.Info("Once threshold is met, anyone can execute with:")
		appUI.Info("  jarvis safe execute %s 0x%s", safeContract.Address, ethcommon.Bytes2Hex(hash[:]))
	},
}

var approveSafeCmd = &cobra.Command{
	Use:   "approve",
	Short: "Off-chain approve a pending Safe transaction (adds your signature to the service)",
	Long: `Sign the EIP-712 safeTxHash of a pending Safe transaction and
submit your signature to the Safe Transaction Service. Identify the
pending tx by:

  - a Safe-app URL (the easiest form for non-CLI signers):
      jarvis safe approve "https://app.safe.global/transactions/tx?id=multisig_<safe>_<hash>&safe=eth:<safe>"

  - the safe address followed by a safeTxHash or SafeTx nonce:
      jarvis safe approve <safe> <safeTxHash|nonce>

If your approval brings the signature count to or above the Safe's
threshold, jarvis automatically chains an execTransaction in the same
invocation so you don't have to run a second command. Pass --no-execute
to opt out (the typical use case is when you want a different EOA to
pay for execution gas).
`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("usage: jarvis safe approve <safe-or-url> [safeTxHash|nonce]")
		}
		return cmdutil.CommonSafeTxPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		safeContract := tc.Safe

		appUI.Section("Safe info")
		showSafeInfo(safeContract)

		identifier, err := pickPendingTxIdentifier(tc, args)
		if err != nil {
			appUI.Error("%s", err)
			return
		}
		pending, err := resolvePendingTx(tc, identifier)
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

		expected := pending.SafeTx.SafeTxHash(domainSep)
		if expected != pending.SafeTxHash {
			appUI.Error(
				"safeTxHash from service (0x%s) doesn't match locally recomputed hash (0x%s); refusing to sign",
				ethcommon.Bytes2Hex(pending.SafeTxHash[:]),
				ethcommon.Bytes2Hex(expected[:]),
			)
			return
		}

		showSafeTxToConfirm(pending.SafeTx, pending.SafeTxHash, &tc)
		showSafeSigners("Existing signatures", pending.Sigs)

		me := ethcommon.HexToAddress(tc.From)
		for _, s := range pending.Sigs {
			if s.Owner == me {
				appUI.Warn("You (%s) have already signed this transaction.", me.Hex())
				return
			}
		}

		if !config.YesToAllPrompt && !appUI.Confirm("Sign and submit your approval?", true) {
			appUI.Warn("Aborted by user.")
			return
		}

		appUI.Info("Unlock your wallet and sign the EIP-712 safeTxHash now...")
		account, err := accounts.UnlockAccount(tc.FromAcc)
		if err != nil {
			appUI.Error("Couldn't unlock wallet: %s", err)
			return
		}

		structHash := pending.SafeTx.StructHash()
		sig, err := account.SignSafeHash(domainSep, structHash)
		if err != nil {
			appUI.Error("Couldn't sign safeTxHash: %s", err)
			return
		}

		if err := tc.Collector.Confirm(pending.SafeTxHash, me, sig); err != nil {
			appUI.Error("Submitting confirmation to Safe Transaction Service failed: %s", err)
			return
		}

		appUI.Success("Confirmation submitted.")
		totalSigs := len(pending.Sigs) + 1
		appUI.Info("Total signatures now: %d", totalSigs)

		threshold, err := safeContract.Threshold()
		if err != nil {
			appUI.Warn("Couldn't read safe threshold post-approval: %s", err)
			return
		}
		if uint64(totalSigs) < threshold {
			appUI.Info(
				"Need %d more approval(s). Once threshold is met, any owner can run:",
				threshold-uint64(totalSigs),
			)
			appUI.Info(
				"  jarvis safe execute %s 0x%s",
				safeContract.Address, ethcommon.Bytes2Hex(pending.SafeTxHash[:]),
			)
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
			appUI.Info(
				"  jarvis safe execute %s 0x%s",
				safeContract.Address, ethcommon.Bytes2Hex(pending.SafeTxHash[:]),
			)
			return
		}
		if !config.YesToAllPrompt && !appUI.Confirm("Broadcast execTransaction now?", true) {
			appUI.Warn("Skipping execution. Run later with:")
			appUI.Info(
				"  jarvis safe execute %s 0x%s",
				safeContract.Address, ethcommon.Bytes2Hex(pending.SafeTxHash[:]),
			)
			return
		}

		augmented := *pending
		augmented.Sigs = append(append([]safe.OwnerSig{}, pending.Sigs...), safe.OwnerSig{
			Owner: me,
			Sig:   sig,
		})
		runSafeExecute(tc, safeContract, &augmented, domainSep)
	},
}

var executeSafeCmd = &cobra.Command{
	Use:   "execute",
	Short: "Execute a Safe transaction whose signatures meet the threshold",
	Long: `Fetch a pending Safe transaction, assemble its signatures into the
format Safe.execTransaction expects, and broadcast the on-chain execution
from --from (or the single matching owner you have a wallet for).

The pending tx can be identified by:

  - a Safe-app URL:
      jarvis safe execute "https://app.safe.global/transactions/tx?id=multisig_<safe>_<hash>&safe=eth:<safe>"

  - the safe address followed by a safeTxHash or SafeTx nonce:
      jarvis safe execute <safe> <safeTxHash|nonce>
`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("usage: jarvis safe execute <safe-or-url> [safeTxHash|nonce]")
		}
		return cmdutil.CommonSafeTxPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)
		safeContract := tc.Safe

		appUI.Section("Safe info")
		showSafeInfo(safeContract)

		identifier, err := pickPendingTxIdentifier(tc, args)
		if err != nil {
			appUI.Error("%s", err)
			return
		}
		pending, err := resolvePendingTx(tc, identifier)
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

		threshold, _ := safeContract.Threshold()
		pending, err := tc.Collector.ListPending(ethcommon.HexToAddress(safeContract.Address))
		if err != nil {
			appUI.Error("Couldn't fetch the pending queue: %s", err)
			return
		}

		appUI.Section(fmt.Sprintf("Pending Safe transactions: %d", len(pending)))
		if len(pending) == 0 {
			appUI.Info("Queue is empty. Use `jarvis safe init` to propose a new tx.")
			return
		}
		for i, p := range pending {
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

		identifier, err := pickPendingTxIdentifier(tc, args)
		if err != nil {
			appUI.Error("%s", err)
			return
		}
		pending, err := resolvePendingTx(tc, identifier)
		if err != nil {
			appUI.Error("%s", err)
			return
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
				"  jarvis safe execute %s 0x%s",
				safeContract.Address, ethcommon.Bytes2Hex(pending.SafeTxHash[:]),
			)
		default:
			needed := uint64(0)
			if threshold > uint64(len(pending.Sigs)) {
				needed = threshold - uint64(len(pending.Sigs))
			}
			appUI.Info("Status: pending — needs %d more approval(s).", needed)
			appUI.Info(
				"  jarvis safe approve %s 0x%s",
				safeContract.Address, ethcommon.Bytes2Hex(pending.SafeTxHash[:]),
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

// safeBatchResult is the per-ref outcome of `jarvis safe bapprove`.
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

// bapproveSafeCmd takes a free-form list of Safe references (URLs, raw
// `multisig_<safe>_<hash>` tokens, or `<chain>:<safe>:<hash>` triples) and
// approves each one in turn, mirroring `jarvis msig bapprove`. Anything we
// can't parse, find, or sign for is recorded in the final summary so the
// operator can see at a glance which approvals went through.
var bapproveSafeCmd = &cobra.Command{
	Use:   "bapprove",
	Short: "Approve multiple pending Safe transactions in one shot",
	Long: `Process a list of Safe transaction references and approve each.
Each whitespace- or comma-separated token may be:

  - a Safe-app URL: https://app.safe.global/transactions/tx?id=multisig_<safe>_<hash>&safe=<chain>:<safe>
  - a bare multisig token: multisig_<safe>_<hash>
  - an EIP-3770 chain prefix + hash: <chain>:<safe>:<hash>

A summary table is printed at the end. With --json-output-file, the same
data is also written as JSON.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			appUI.Error("Please provide one or more Safe tx references.")
			return
		}
		raw := strings.Join(args, " ")
		refs := scanForSafeRefs(raw)
		if len(refs) == 0 {
			appUI.Error("No Safe tx references parsed from input.")
			return
		}

		total := len(refs)
		results := make([]safeBatchResult, 0, total)

		appUI.Section(fmt.Sprintf("Batch Approve: %d Safe transactions", total))

		for i, ref := range refs {
			r := safeBatchResult{ref: ref.original}
			appUI.Info("")
			appUI.Critical("━━━ [%d/%d] %s ━━━", i+1, total, ref.original)

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
	},
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
			ref.ChainID = safe.SafeShortNameChainID[ref.ChainShortName]
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

// approveSafeRef performs the same logical steps as `jarvis safe approve`
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
	matchingOwners := 0
	for _, o := range owners {
		acc, err := accounts.GetAccount(o)
		if err == nil {
			fromAcc = acc
			matchingOwners++
		}
	}
	if matchingOwners == 0 {
		res.status = "skipped"
		res.reason = "no local wallet is an owner of this safe"
		appUI.Warn("%s", res.reason)
		return res
	}
	if matchingOwners > 1 && config.From == "" {
		res.status = "skipped"
		res.reason = "multiple local owner wallets; pass --from to disambiguate"
		appUI.Warn("%s", res.reason)
		return res
	}
	if config.From != "" {
		acc, err := accounts.GetAccount(config.From)
		if err != nil {
			res.status = "failed"
			res.reason = fmt.Sprintf("--from %q: %s", config.From, err)
			appUI.Error("%s", res.reason)
			return res
		}
		isOwner := false
		for _, o := range owners {
			if strings.EqualFold(o, acc.Address) {
				isOwner = true
				break
			}
		}
		if !isOwner {
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
	if expected := pending.SafeTx.SafeTxHash(domainSep); expected != pending.SafeTxHash {
		res.status = "failed"
		res.reason = "service safeTxHash doesn't match locally recomputed hash"
		appUI.Error("%s", res.reason)
		return res
	}

	me := ethcommon.HexToAddress(fromAcc.Address)
	for _, s := range pending.Sigs {
		if s.Owner == me {
			res.status = "skipped"
			res.reason = fmt.Sprintf("%s already signed", me.Hex())
			appUI.Warn("%s", res.reason)
			return res
		}
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

	if !config.YesToAllPrompt && !appUI.Confirm("Sign and submit your approval?", true) {
		res.status = "skipped"
		res.reason = "user aborted"
		return res
	}

	appUI.Info("Unlock %s and sign the EIP-712 safeTxHash now...", fromAcc.Address)
	account, err := accounts.UnlockAccount(fromAcc)
	if err != nil {
		res.status = "failed"
		res.reason = fmt.Sprintf("unlock wallet: %s", err)
		appUI.Error("%s", res.reason)
		return res
	}

	structHash := pending.SafeTx.StructHash()
	sig, err := account.SignSafeHash(domainSep, structHash)
	if err != nil {
		res.status = "failed"
		res.reason = fmt.Sprintf("sign safeTxHash: %s", err)
		appUI.Error("%s", res.reason)
		return res
	}

	if err := collector.Confirm(pending.SafeTxHash, me, sig); err != nil {
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
		appUI.Info("Skipping execution. Run later with `jarvis safe execute ...`.")
		return res
	}

	augmented := *pending
	augmented.Sigs = append(append([]safe.OwnerSig{}, pending.Sigs...), safe.OwnerSig{
		Owner: me, Sig: sig,
	})
	runSafeExecute(tc, safeContract, &augmented, domainSep)
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

	if config.GasPrice == 0 {
		tc.GasPrice, err = r.RecommendedGasPrice()
		if err != nil {
			return cmdutil.TxContext{}, fmt.Errorf("recommended gas price: %w", err)
		}
	} else {
		tc.GasPrice = config.GasPrice
	}
	if config.Nonce == 0 {
		tc.Nonce, err = r.GetMinedNonce(tc.From)
		if err != nil {
			return cmdutil.TxContext{}, fmt.Errorf("get nonce: %w", err)
		}
	} else {
		tc.Nonce = config.Nonce
	}
	tc.TxType, err = cmdutil.ValidTxType(r, network)
	if err != nil {
		return cmdutil.TxContext{}, fmt.Errorf("tx type: %w", err)
	}
	if tc.TxType == 2 && config.TipGas == 0 {
		tc.TipGas, _ = r.GetSuggestedGasTipCap()
	} else {
		tc.TipGas = config.TipGas
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

func writeSafeBatchSummaryJSON(path string, results []safeBatchResult) {
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
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		appUI.Error("Couldn't marshal JSON: %s", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		appUI.Error("Couldn't write JSON file: %s", err)
		return
	}
	appUI.Success("Summary written to %s", path)
}

// runSafeExecute drives the on-chain execTransaction call for a Safe
// transaction whose signatures have already been collected. It is shared
// between `jarvis safe execute` and the auto-execute path of `jarvis safe
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
	appUI.Section("Safe transaction details")

	// Best-effort: ask the network's block explorer for the destination's
	// contract name (and follow proxies to the implementation). Populates
	// the local cache so the addrbook resolver below can render a real
	// name instead of "unknown" for contracts that aren't in jarvis's
	// embedded address DB.
	util.PrefetchContractName(stx.To.Hex(), config.Network())

	// Resolve the destination through the address book so users see a name
	// (or "unknown" highlighted) instead of an opaque hex string.
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
	destAbi, err := tc.Resolver.ConfigToABI(
		stx.To.Hex(), config.ForceERC20ABI, config.CustomABI, config.Network(),
	)
	if err != nil {
		appUI.Warn("Couldn't resolve ABI of destination %s: %s", stx.To.Hex(), err)
		appUI.Critical("Data (%d bytes): 0x%s", len(stx.Data), ethcommon.Bytes2Hex(stx.Data))
		return
	}
	util.AnalyzeMethodCallAndPrint(
		appUI,
		tc.Analyzer,
		stx.Value,
		stx.To.Hex(),
		stx.Data,
		map[string]*abi.ABI{strings.ToLower(stx.To.Hex()): destAbi},
		config.Network(),
	)
}

// showSafeSigners renders the list of owners that have already signed,
// resolving each address through the jarvis address book so names show up
// the same way `jarvis msig` displays confirmation lists.
func showSafeSigners(label string, sigs []safe.OwnerSig) {
	if len(sigs) == 0 {
		appUI.Info("%s: (none yet)", label)
		return
	}
	appUI.Info("%s (%d):", label, len(sigs))
	for i, s := range sigs {
		jarvisAddr := util.GetJarvisAddress(s.Owner.Hex(), config.Network())
		appUI.Info("  %d. %s", i+1, appUI.Style(util.StyledAddress(jarvisAddr)))
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
// Honors --safe-nonce when set, else combines the on-chain nonce with the
// service's pending queue so multiple in-flight proposals don't collide.
func nextSafeNonce(s *safe.SafeContract, c safe.SignatureCollector) (uint64, error) {
	if safeNonceOverride != 0 {
		return safeNonceOverride, nil
	}
	onchain, err := s.Nonce()
	if err != nil {
		return 0, fmt.Errorf("reading on-chain nonce: %w", err)
	}
	next := onchain
	// Walk forward until we find a free slot. The service is authoritative
	// for "is there an in-flight proposal at nonce N?" so we just probe.
	for i := uint64(0); i < 64; i++ {
		pending, err := c.FindByNonce(ethcommon.HexToAddress(s.Address), next)
		if err != nil {
			// On the first iteration treat lookup errors as fatal; otherwise
			// assume we've walked past the queue.
			if i == 0 {
				return 0, fmt.Errorf("checking pending queue at nonce %d: %w", next, err)
			}
			break
		}
		if pending == nil {
			return next, nil
		}
		next++
	}
	return next, nil
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
			"no pending Safe transactions found for %s. Initiate one with `jarvis safe init`, or pass a safeTxHash / nonce explicitly.",
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
	if strings.HasPrefix(strings.ToLower(identifier), "0x") && len(identifier) == 66 {
		var hash [32]byte
		copy(hash[:], ethcommon.FromHex(identifier))
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

func init() {
	initSafeCmd.Flags().Float64VarP(
		&config.MsigValue, "msig-value", "V", 0,
		"Native-token amount the Safe should forward (in eth/bnb/matic units).",
	)
	initSafeCmd.Flags().StringVarP(
		&config.MsigTo, "msig-to", "j", "",
		"Target address the Safe will call. Address or jarvis name.",
	)
	initSafeCmd.Flags().Uint64VarP(
		&config.MethodIndex, "method-index", "M", 0,
		"1-based index of the target method (alphabetically sorted).",
	)
	initSafeCmd.Flags().BoolVarP(
		&config.NoFuncCall, "no-func-call", "N", false,
		"True: send empty calldata to the Safe destination.",
	)
	initSafeCmd.Flags().StringVarP(
		&config.PrefillStr, "prefills", "I", "",
		"Prefill params separated by '|'. '?' means prompt.",
	)
	initSafeCmd.Flags().Uint64Var(
		&safeNonceOverride, "safe-nonce", 0,
		"Override the SafeTx nonce. Default: on-chain nonce + length of pending queue.",
	)
	initSafeCmd.MarkFlagRequired("msig-to")

	approveSafeCmd.Flags().BoolVar(
		&safeNoExecute, "no-execute", false,
		"Don't auto-execute even when this approval reaches the threshold.",
	)

	writeCmds := []*cobra.Command{
		initSafeCmd,
		approveSafeCmd,
		executeSafeCmd,
	}
	for _, c := range writeCmds {
		AddCommonFlagsToTransactionalCmds(c)
		c.Flags().StringVarP(
			&config.RawValue, "amount", "v", "0",
			"Native-token amount to attach to the EOA tx that drives the Safe (almost always 0).",
		)
		c.PersistentFlags().BoolVarP(
			&config.ForceERC20ABI, "erc20-abi", "e", false,
			"Use ERC20 ABI for the target where possible.",
		)
		c.PersistentFlags().StringVarP(
			&config.CustomABI, "abi", "c", "",
			"Custom ABI for the target. Address, file path or URL.",
		)
		c.PersistentFlags().BoolVarP(
			&config.YesToAllPrompt, "auto-yes", "y", false,
			"Don't prompt Yes/No before signing.",
		)
	}

	safeCmd.AddCommand(initSafeCmd)
	safeCmd.AddCommand(approveSafeCmd)
	safeCmd.AddCommand(executeSafeCmd)
	safeCmd.AddCommand(summarySafeCmd)
	safeCmd.AddCommand(infoSafeCmd)
	safeCmd.AddCommand(govSafeCmd)
	safeCmd.AddCommand(bapproveSafeCmd)
	rootCmd.AddCommand(safeCmd)
}
