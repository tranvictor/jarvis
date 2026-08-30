package util

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/spf13/cobra"

	jtypes "github.com/tranvictor/jarvis/accounts/types"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
	utilreader "github.com/tranvictor/jarvis/util/reader"
)

// showNodeErrorGuidance prints a structured diagnostic block when an RPC call
// fails, suggesting how the user can inspect and fix their node configuration.
func showNodeErrorGuidance(u ui.UI, network networks.Network) {
	u.Warn("Could not reach any RPC node for network %q.", network.GetName())
	u.Info("  What to try:")
	u.Info("    Check your nodes:  jarvis node list %s", network.GetName())
	u.Info("    Test connectivity: jarvis node test %s", network.GetName())
	u.Info("    Add a custom node: jarvis node add %s <name> <url>", network.GetName())
}

// CommonFunctionCallPreprocess populates a TxContext with the values derivable
// from the function-call arguments: target address, parsed value, prefill params,
// and an optional TxInfo when the argument is a tx hash. It attaches the context
// to the cobra command so Run functions can retrieve it via TxContextFrom.
func CommonFunctionCallPreprocess(u ui.UI, cmd *cobra.Command, args []string) (err error) {
	// Peek at args[0] for a network-prefixed tx hash (e.g. "base:0x...").
	// The network must be resolved BEFORE the reader/analyzer are built,
	// otherwise we bind to the default network and fail to fetch the tx.
	// An explicit -N/--network flag still wins if the user passed it.
	if len(args) > 0 && !cmd.Flags().Changed("network") {
		if nwks, txs := ScanForTxs(args[0]); len(txs) > 0 && nwks[0] != "" {
			config.NetworkString = nwks[0]
		}
	}

	if err = config.SetNetwork(config.NetworkString); err != nil {
		return err
	}
	u.Info("Network: %s", config.Network().GetName())

	tc := TxContext{}

	r, err := util.EthReader(config.Network())
	if err != nil {
		return fmt.Errorf("couldn't connect to blockchain: %w", err)
	}
	tc.Reader = r
	tc.Analyzer = txanalyzer.NewGenericAnalyzer(r, config.Network())
	tc.Resolver = DefaultABIResolver{}

	prefillStr := strings.Trim(config.PrefillStr, " ")
	if prefillStr != "" {
		tc.PrefillMode = true
		tc.PrefillParams = strings.Split(prefillStr, "|")
		for i := range tc.PrefillParams {
			tc.PrefillParams[i] = strings.Trim(tc.PrefillParams[i], " ")
		}
	}

	tc.Value, err = jarviscommon.FloatStringToBig(config.RawValue, 18)
	if err != nil {
		return fmt.Errorf("couldn't parse -v param: %s", err)
	}
	if tc.Value.Cmp(big.NewInt(0)) < 0 {
		return fmt.Errorf("-v param can't be negative")
	}

	if len(args) == 0 {
		tc.To = "" // contract creation tx
	} else {
		tc.To, _, err = util.GetAddressFromString(args[0])
		if err != nil {
			_, txs := ScanForTxs(args[0])
			if len(txs) == 0 {
				return fmt.Errorf("can't interpret the contract address")
			}
			config.Tx = txs[0]

			txinfo, err := r.TxInfoFromHash(config.Tx)
			if err != nil {
				return fmt.Errorf("couldn't get tx info from the blockchain: %w", err)
			}
			tc.TxInfo = &txinfo
			tc.To = tc.TxInfo.Tx.To().Hex()
		}
	}

	cmd.SetContext(WithTxContext(cmd.Context(), tc))
	return nil
}

// CommonNetworkPreprocess sets up the network and injects Reader, Analyzer,
// and Resolver into TxContext. It does not resolve any positional argument as a
// contract address, making it suitable for commands that operate on arbitrary
// tx hashes or other non-address arguments (e.g. the "info" command).
func CommonNetworkPreprocess(u ui.UI, cmd *cobra.Command, args []string) error {
	// Peek at args for a network-prefixed tx hash (e.g. "mainnet:0x...").
	// The network must be resolved BEFORE the reader/analyzer are built,
	// otherwise we bind to the default network and fetch the wrong chain.
	// An explicit -N/--network flag still wins if the user passed it.
	if len(args) > 0 && !cmd.Flags().Changed("network") {
		para := strings.Join(args, " ")
		if nwks, txs := ScanForTxs(para); len(txs) > 0 && nwks[0] != "" {
			config.NetworkString = nwks[0]
		}
	}

	if err := config.SetNetwork(config.NetworkString); err != nil {
		return err
	}
	u.Info("Network: %s", config.Network().GetName())

	tc := TxContext{}

	r, err := util.EthReader(config.Network())
	if err != nil {
		return fmt.Errorf("couldn't connect to blockchain: %w", err)
	}
	tc.Reader = r
	tc.Analyzer = txanalyzer.NewGenericAnalyzer(r, config.Network())
	tc.Resolver = DefaultABIResolver{}

	cmd.SetContext(WithTxContext(cmd.Context(), tc))
	return nil
}

// CommonSendPreprocess is a lightweight preprocess for the send command. It
// initialises the network and injects an EthReader and Broadcaster into
// TxContext so sendCmd.Run can use them without a live-node dependency in
// tests. Gas, nonce, and TxType resolution is deliberately left to Run
// because they depend on the specific token/amount being sent.
func CommonSendPreprocess(u ui.UI, cmd *cobra.Command, args []string) error {
	if err := config.SetNetwork(config.NetworkString); err != nil {
		return err
	}
	u.Info("Network: %s", config.Network().GetName())

	tc := TxContext{}

	r, err := util.EthReader(config.Network())
	if err != nil {
		return fmt.Errorf("couldn't connect to blockchain: %w", err)
	}
	tc.Reader = r
	tc.Analyzer = txanalyzer.NewGenericAnalyzer(r, config.Network())
	tc.Resolver = DefaultABIResolver{}

	bc, err := util.EthBroadcaster(config.Network())
	if err != nil {
		return fmt.Errorf("couldn't connect to broadcaster: %w", err)
	}
	tc.Broadcaster = bc

	cmd.SetContext(WithTxContext(cmd.Context(), tc))
	return nil
}

// CommonTxPreprocess extends CommonFunctionCallPreprocess by also resolving the
// signing account and fetching gas/nonce parameters. It overwrites the TxContext
// attached to cmd by CommonFunctionCallPreprocess.
func CommonTxPreprocess(u ui.UI, cmd *cobra.Command, args []string) (err error) {
	if err = CommonFunctionCallPreprocess(u, cmd, args); err != nil {
		return err
	}

	tc, _ := TxContextFrom(cmd)

	a, err := util.GetABI(tc.To, config.Network())
	if err != nil {
		if config.ForceERC20ABI {
			a = jarviscommon.GetERC20ABI()
		} else if config.CustomABI != "" {
			a, err = util.ReadCustomABI(tc.To, config.CustomABI, config.Network())
			if err != nil {
				return fmt.Errorf("reading custom abi failed: %w", err)
			}
		}
	}

	var fromAcc jtypes.AccDesc
	if config.From == "" {
		if !classicMultisigOwnerPickEligible(config.Network(), tc.To, a, EthReaderOf(tc.Reader)) {
			return fmt.Errorf("please specify the signing wallet with --from")
		}
		multisigContract, err := msig.NewMultisigContract(tc.To, config.Network(), msig.WithReader(EthReaderOf(tc.Reader)))
		if err != nil {
			return err
		}
		owners, err := multisigContract.Owners()
		if err != nil {
			return fmt.Errorf("getting msig owners failed: %w", err)
		}

		fromAcc, _, err = PickLocalOwner(owners, config.From, OwnerRequireUnique)
		if errors.Is(err, ErrNoLocalOwner) {
			return fmt.Errorf(
				"you don't have any wallet which is this multisig signer. please jarvis wallet add to add the wallet",
			)
		}
		if errors.Is(err, ErrMultipleLocalOwners) {
			return fmt.Errorf(
				"you have many wallets that are this multisig signers. please specify only 1",
			)
		}
		if err != nil {
			return err
		}
	} else {
		fromAcc, _, err = ResolveAccount(tc.Resolver, config.From)
		if err != nil {
			return err
		}
	}

	tc.FromAcc = fromAcc
	tc.From = fromAcc.Address

	if err := FillSigningTxParams(u, &tc, config.Network()); err != nil {
		return err
	}

	cmd.SetContext(WithTxContext(cmd.Context(), tc))
	return nil
}

// CommonSafeReadPreprocess is the read-only counterpart to
// CommonSafeTxPreprocess: it resolves the Safe address (accepting URL /
// EIP-3770 / bare address forms) and wires Reader, Analyzer, Resolver,
// SafeContract and a Safe Transaction Service Collector into TxContext —
// but does NOT pick a signing wallet, fetch gas/nonce/tx-type, or build a
// broadcaster. Use this for inspection commands (`summary`, `info`, `gov`)
// where requiring an owner wallet would be unfriendly.
func CommonSafeReadPreprocess(u ui.UI, cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("please specify the safe address (or URL) as the first argument")
	}
	ref, err := preResolveMultisigArg(u, cmd, args)
	if err != nil {
		return err
	}

	if err := CommonFunctionCallPreprocess(u, cmd, args); err != nil {
		return err
	}
	tc, _ := TxContextFrom(cmd)
	tc.SafeAppRef = ref

	if tc.To == "" {
		return fmt.Errorf("please specify the safe address as the first argument")
	}

	safeContract, err := safe.NewSafeContract(tc.To, config.Network(), safe.WithReader(EthReaderOf(tc.Reader)))
	if err != nil {
		return fmt.Errorf("couldn't init safe reader: %w", err)
	}
	if _, err := safeContract.Owners(); err != nil {
		return fmt.Errorf(
			"couldn't read safe owners — %s does not appear to be a Gnosis Safe: %w",
			tc.To, err,
		)
	}
	tc.Safe = safeContract

	wireOptionalCollector(u, &tc)

	cmd.SetContext(WithTxContext(cmd.Context(), tc))
	return nil
}

// CommonSafeTxPreprocess prepares a TxContext for Gnosis Safe commands.
//
// It expects args[0] to identify a Gnosis Safe contract via any of:
//   - a bare 0x-prefixed address ("0x71f8...")
//   - a jarvis address book name ("my-treasury")
//   - an EIP-3770 short reference ("eth:0x71f8...")
//   - a Safe-app web URL ("https://app.safe.global/transactions/tx?id=...")
//
// When a URL or EIP-3770 reference carries chain information, the network
// is auto-selected (unless --network was passed explicitly). When the URL
// also embeds a safeTxHash (e.g. transaction-detail pages), the parsed
// SafeAppRef is attached to TxContext so approve/execute can use it without
// a positional safeTxHash argument.
//
// The function then:
//  1. Resolves the network and Safe address via the standard preprocess.
//  2. Builds a SafeContract reader and verifies that the on-chain ABI
//     matches the Safe shape (so we don't operate on random addresses).
//  3. Picks the signing wallet — when --from is empty, looks for a single
//     local wallet that is also an owner of the Safe.
//  4. Resolves gas / nonce / tx type for any future on-chain transactions
//     (e.g. execTransaction) so callers can reuse SignAndBroadcast.
//  5. Wires a SignatureCollector backed by the Safe Transaction Service
//     for off-chain signature exchange.
func CommonSafeTxPreprocess(u ui.UI, cmd *cobra.Command, args []string) error {
	// Step 0: try to recognise a Safe-app URL / EIP-3770 reference in
	// args[0] BEFORE the inner preprocess looks at it.
	var ref *safe.SafeAppRef
	if len(args) > 0 {
		var err error
		ref, err = preResolveMultisigArg(u, cmd, args)
		if err != nil {
			return err
		}
	}

	if err := CommonFunctionCallPreprocess(u, cmd, args); err != nil {
		return err
	}
	tc, _ := TxContextFrom(cmd)
	tc.SafeAppRef = ref

	if tc.To == "" {
		return fmt.Errorf("please specify the safe address as the first argument")
	}

	// The explorer-side ABI is informational only. Almost every Safe is
	// deployed as a GnosisSafeProxy whose verified ABI is just a fallback
	// + constructor — that ABI will never satisfy IsGnosisSafe even though
	// the contract behaves like a Safe via DELEGATECALL. The authoritative
	// check is the on-chain getOwners() probe a few lines below.
	if a, abiErr := util.GetABI(tc.To, config.Network()); abiErr == nil {
		if !safe.IsGnosisSafe(a) {
			u.Info(
				"Note: explorer ABI for %s is not a direct Safe ABI (likely a proxy); will verify via on-chain getOwners().",
				tc.To,
			)
		}
	} else {
		u.Warn(
			"Couldn't fetch ABI for %s (%s); will probe on-chain with the minimal Safe ABI.",
			tc.To, abiErr,
		)
	}

	safeContract, err := safe.NewSafeContract(tc.To, config.Network(), safe.WithReader(EthReaderOf(tc.Reader)))
	if err != nil {
		return fmt.Errorf("couldn't init safe reader: %w", err)
	}
	owners, err := safeContract.Owners()
	if err != nil {
		return fmt.Errorf(
			"couldn't read safe owners — %s does not appear to be a Gnosis Safe: %w",
			tc.To, err,
		)
	}
	tc.Safe = safeContract

	var fromAcc jtypes.AccDesc
	if config.From == "" {
		fromAcc, _, err = PickLocalOwner(owners, config.From, OwnerRequireUnique)
		if errors.Is(err, ErrNoLocalOwner) {
			return fmt.Errorf(
				"you don't have any wallet which is an owner of this Safe; please run `jarvis wallet add` first",
			)
		}
		if errors.Is(err, ErrMultipleLocalOwners) {
			return fmt.Errorf(
				"you have multiple wallets that are owners of this Safe; please specify exactly one with --from",
			)
		}
		if err != nil {
			return err
		}
	} else {
		fromAcc, _, err = ResolveAccount(tc.Resolver, config.From)
		if err != nil {
			return err
		}
		if !IsAmongOwners(owners, fromAcc.Address) {
			return fmt.Errorf(
				"%s is not an owner of Safe %s",
				fromAcc.Address, tc.To,
			)
		}
	}
	tc.FromAcc = fromAcc
	tc.From = fromAcc.Address

	if err := FillSigningTxParams(u, &tc, config.Network()); err != nil {
		return err
	}

	wireOptionalCollector(u, &tc)

	cmd.SetContext(WithTxContext(cmd.Context(), tc))
	return nil
}

// wireOptionalCollector attaches a Safe Transaction Service collector when
// the chain has a configured URL. Missing service is a warning, not a
// preprocess failure: inspection and --safe-tx-file / --approve-onchain
// paths still work without it.
func wireOptionalCollector(u ui.UI, tc *TxContext) {
	collector, err := safe.NewTxServiceCollector(config.Network().GetChainID())
	if err == nil {
		tc.Collector = collector
		return
	}
	u.Warn(
		"Safe Transaction Service is not configured for chain %d: %s",
		config.Network().GetChainID(), err,
	)
	u.Warn(
		"Set SAFE_TX_SERVICE_URL_%d (or a global SAFE_TX_SERVICE_URL) to use a self-hosted deployment,",
		config.Network().GetChainID(),
	)
	u.Warn(
		"or pass --safe-tx-file / --approve-onchain to operate without the service.",
	)
}

// preResolveMultisigArg normalises args[0] for the unified multisig
// preprocesses. It handles three cases identically to the Safe-only path:
//
//   - bare addresses and jarvis names: passed through unchanged
//   - Safe-app web URLs / EIP-3770 short refs: rewritten to a bare address
//     in args[0], with config.NetworkString auto-set from the chain hint
//   - the network is preserved when --network was passed explicitly
//
// It returns any SafeAppRef parsed out of the URL so a downstream
// preprocess can attach it to TxContext.SafeAppRef even when the chosen
// dispatch path is the classic one (it's a no-op there).
func preResolveMultisigArg(u ui.UI, cmd *cobra.Command, args []string) (*safe.SafeAppRef, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("please specify the multisig address as the first argument")
	}
	r, ok := safe.ParseSafeAppURL(args[0])
	if !ok {
		return nil, nil
	}
	args[0] = r.SafeAddress.Hex()
	if r.ChainID != 0 && !cmd.Flags().Changed("network") {
		if n, err := networks.GetNetworkByID(r.ChainID); err == nil {
			config.NetworkString = n.GetName()
		} else {
			u.Warn(
				"URL refers to chain id %d (%s) which jarvis does not have a built-in network for; falling back to --network=%s.",
				r.ChainID, r.ChainShortName, config.NetworkString,
			)
		}
	}
	return r, nil
}

// applyMultisigArgNetworkHint sets config.NetworkString from a
// network-prefixed first argument (e.g. "mainnet:0x…") when the user
// did not pass -k/--network explicitly.
func applyMultisigArgNetworkHint(cmd *cobra.Command, args []string) {
	if len(args) == 0 || cmd.Flags().Changed("network") {
		return
	}
	if nwks, txs := ScanForTxs(args[0]); len(txs) > 0 && nwks[0] != "" {
		config.NetworkString = nwks[0]
	}
}

// classicMultisigOwnerPickEligible reports whether tc.To is a Gnosis Classic
// multisig so an empty --from can be resolved by picking the sole local owner.
// Explorer ABIs are often incomplete (proxy/factory shapes), so we fall back
// to the same on-chain NOTransactions probe used by DetectMultisigType.
func classicMultisigOwnerPickEligible(network networks.Network, to string, contractABI *abi.ABI, r *utilreader.EthReader) bool {
	if contractABI != nil {
		if ok, err := util.IsGnosisMultisig(contractABI); err == nil && ok {
			return true
		}
	}
	mc, err := msig.NewMultisigContract(to, network, msig.WithReader(r))
	if err != nil {
		return false
	}
	_, err = mc.NOTransactions()
	return err == nil
}

// resolveMultisigProbeAddress turns args[0] into the contract address
// DetectMultisigType should probe. Jarvis names and bare addresses pass
// through; Gnosis Classic init tx hashes (optionally network-prefixed)
// are resolved via the tx's `to` field, matching bapprove and the
// legacy single-argument approve/execute flow.
func resolveMultisigProbeAddress(arg string) (string, error) {
	addr, _, err := DefaultABIResolver{}.GetAddressFromString(arg)
	if err == nil {
		return addr, nil
	}

	_, txs := ScanForTxs(arg)
	if len(txs) == 0 {
		return arg, nil
	}

	txHash := txs[0]
	if !strings.HasPrefix(txHash, "0x") {
		txHash = "0x" + txHash
	}

	r, err := util.EthReader(config.Network())
	if err != nil {
		return "", fmt.Errorf("couldn't connect to blockchain: %w", err)
	}
	txinfo, err := r.TxInfoFromHash(txHash)
	if err != nil {
		return "", fmt.Errorf("couldn't get tx info from the blockchain: %w", err)
	}
	if txinfo.Tx.To() == nil {
		return "", fmt.Errorf("%q is a contract-creation tx, not a multisig init tx", arg)
	}
	return txinfo.Tx.To().Hex(), nil
}

// CommonMultisigReadPreprocess is the unified read-only preprocess for
// `jarvis msig` inspection commands (info / summary / gov). It accepts
// the same first-argument shapes as the Safe-only equivalent (bare
// address, jarvis name, EIP-3770 ref, Safe-app URL) and dispatches to
// the Safe or Classic read setup based on an on-chain probe of the
// resolved address. The detected MultisigType is stored on TxContext so
// the dispatching command's Run can route accordingly.
//
// For Classic addresses it falls back to CommonNetworkPreprocess (which
// is what classic msig info/gov/summary use today). For Safe addresses
// it calls CommonSafeReadPreprocess so SafeContract + Collector are wired.
func CommonMultisigReadPreprocess(u ui.UI, cmd *cobra.Command, args []string) error {
	ref, err := preResolveMultisigArg(u, cmd, args)
	if err != nil {
		return err
	}
	applyMultisigArgNetworkHint(cmd, args)
	if err := config.SetNetwork(config.NetworkString); err != nil {
		return err
	}

	addr, err := resolveMultisigProbeAddress(args[0])
	if err != nil {
		return err
	}

	typ, err := DetectMultisigType(config.Network(), addr)
	if err != nil {
		return err
	}

	switch typ {
	case MultisigSafe:
		if err := CommonSafeReadPreprocess(u, cmd, args); err != nil {
			return err
		}
	case MultisigClassic:
		if err := CommonNetworkPreprocess(u, cmd, args); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown multisig type for %s", addr)
	}

	tc, _ := TxContextFrom(cmd)
	tc.MultisigType = typ
	if ref != nil {
		tc.SafeAppRef = ref
	}
	cmd.SetContext(WithTxContext(cmd.Context(), tc))
	return nil
}

// CommonMultisigTxPreprocess is the transactional twin of
// CommonMultisigReadPreprocess: it picks the right preprocessing pipeline
// (Safe-aware vs. Classic-aware) for `jarvis msig` write commands
// (init / approve / execute / revoke). The resolved MultisigType is left
// on TxContext for the dispatcher to inspect.
//
// Note: revoke is classic-only; the Run dispatcher is expected to refuse
// the operation when MultisigType == MultisigSafe with an actionable error.
func CommonMultisigTxPreprocess(u ui.UI, cmd *cobra.Command, args []string) error {
	ref, err := preResolveMultisigArg(u, cmd, args)
	if err != nil {
		return err
	}
	applyMultisigArgNetworkHint(cmd, args)
	if err := config.SetNetwork(config.NetworkString); err != nil {
		return err
	}

	addr, err := resolveMultisigProbeAddress(args[0])
	if err != nil {
		return err
	}

	typ, err := DetectMultisigType(config.Network(), addr)
	if err != nil {
		return err
	}

	switch typ {
	case MultisigSafe:
		if err := CommonSafeTxPreprocess(u, cmd, args); err != nil {
			return err
		}
	case MultisigClassic:
		if err := CommonTxPreprocess(u, cmd, args); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown multisig type for %s", addr)
	}

	tc, _ := TxContextFrom(cmd)
	tc.MultisigType = typ
	if ref != nil {
		tc.SafeAppRef = ref
	}
	cmd.SetContext(WithTxContext(cmd.Context(), tc))
	return nil
}
