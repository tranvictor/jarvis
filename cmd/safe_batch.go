package cmd

// Safe half of `jarvis msig bapprove`: ref scan, per-ref approve, summaries.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"

	jtypes "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util"
)

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
