package cmd

// Safe{Wallet} Transaction Builder JSON parsing for `jarvis msig init`.

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/safe"
)

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
