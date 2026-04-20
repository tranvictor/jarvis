package util

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/accounts"
	jtypes "github.com/tranvictor/jarvis/accounts/types"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
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

	isGnosisMultisig := false
	if err == nil {
		isGnosisMultisig, err = util.IsGnosisMultisig(a)
		if err != nil {
			return fmt.Errorf("checking if the address is gnosis multisig classic failed: %w", err)
		}
	}

	var fromAcc jtypes.AccDesc
	if config.From == "" && isGnosisMultisig {
		multisigContract, err := msig.NewMultisigContract(tc.To, config.Network())
		if err != nil {
			return err
		}
		owners, err := multisigContract.Owners()
		if err != nil {
			return fmt.Errorf("getting msig owners failed: %w", err)
		}

		count := 0
		for _, owner := range owners {
			acc, err := accounts.GetAccount(owner)
			if err == nil {
				fromAcc = acc
				count++
			}
		}
		if count == 0 {
			return fmt.Errorf(
				"you don't have any wallet which is this multisig signer. please jarvis wallet add to add the wallet",
			)
		}
		if count != 1 {
			return fmt.Errorf(
				"you have many wallets that are this multisig signers. please specify only 1",
			)
		}
	} else {
		fromAcc, err = accounts.GetAccount(config.From)
		if err != nil {
			return err
		}
	}

	tc.FromAcc = fromAcc
	tc.From = fromAcc.Address

	// tc.Reader is set by CommonFunctionCallPreprocess; use it directly.
	reader := tc.Reader

	if config.GasPrice == 0 {
		tc.GasPrice, err = reader.RecommendedGasPrice()
		if err != nil {
			showNodeErrorGuidance(u, config.Network())
			return fmt.Errorf("getting recommended gas price failed: %w", err)
		}
	} else {
		tc.GasPrice = config.GasPrice
	}

	if config.Nonce == 0 {
		tc.Nonce, err = reader.GetMinedNonce(tc.From)
		if err != nil {
			showNodeErrorGuidance(u, config.Network())
			return fmt.Errorf("getting nonce failed: %w", err)
		}
	} else {
		tc.Nonce = config.Nonce
	}

	tc.TxType, err = ValidTxType(reader, config.Network())
	if err != nil {
		showNodeErrorGuidance(u, config.Network())
		return fmt.Errorf("couldn't determine proper tx type: %w", err)
	}

	if tc.TxType == types.LegacyTxType && config.TipGas > 0 {
		return fmt.Errorf("we are doing legacy tx hence we ignore tip gas parameter")
	}

	if tc.TxType == types.DynamicFeeTxType {
		if config.TipGas == 0 {
			tc.TipGas, err = reader.GetSuggestedGasTipCap()
			if err != nil {
				showNodeErrorGuidance(u, config.Network())
				return fmt.Errorf("couldn't estimate recommended gas price: %w", err)
			}
		} else {
			tc.TipGas = config.TipGas
		}
	}

	bc, err := util.EthBroadcaster(config.Network())
	if err != nil {
		showNodeErrorGuidance(u, config.Network())
		return fmt.Errorf("couldn't connect to broadcaster: %w", err)
	}
	tc.Broadcaster = bc

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
	var ref *safe.SafeAppRef
	if r, ok := safe.ParseSafeAppURL(args[0]); ok {
		ref = r
		args[0] = r.SafeAddress.Hex()
		if r.ChainID != 0 && !cmd.Flags().Changed("network") {
			if n, err := networks.GetNetworkByID(r.ChainID); err == nil {
				config.NetworkString = n.GetName()
			}
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

	safeContract, err := safe.NewSafeContract(tc.To, config.Network())
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

	collector, err := safe.NewTxServiceCollector(config.Network().GetChainID())
	if err != nil {
		return fmt.Errorf(
			"couldn't init safe transaction service client for chain %d: %w",
			config.Network().GetChainID(), err,
		)
	}
	tc.Collector = collector

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
	// args[0] BEFORE the inner preprocess looks at it. We rewrite args[0]
	// in place to a bare address so all downstream resolution paths work
	// unchanged, and we set config.NetworkString from the chain prefix
	// when the user didn't pass --network explicitly.
	var ref *safe.SafeAppRef
	if len(args) > 0 {
		if r, ok := safe.ParseSafeAppURL(args[0]); ok {
			ref = r
			args[0] = r.SafeAddress.Hex()
			if r.ChainID != 0 && !cmd.Flags().Changed("network") {
				if n, err := networks.GetNetworkByID(r.ChainID); err == nil {
					config.NetworkString = n.GetName()
				} else {
					u.Warn(
						"Safe URL refers to chain id %d (%s) which jarvis doesn't have a built-in network for; falling back to --network=%s. Add a custom network or pass -k explicitly to override.",
						r.ChainID, r.ChainShortName, config.NetworkString,
					)
				}
			}
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

	safeContract, err := safe.NewSafeContract(tc.To, config.Network())
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
		count := 0
		for _, owner := range owners {
			acc, err := accounts.GetAccount(owner)
			if err == nil {
				fromAcc = acc
				count++
			}
		}
		if count == 0 {
			return fmt.Errorf(
				"you don't have any wallet which is an owner of this Safe; please run `jarvis wallet add` first",
			)
		}
		if count > 1 {
			return fmt.Errorf(
				"you have multiple wallets that are owners of this Safe; please specify exactly one with --from",
			)
		}
	} else {
		fromAcc, err = accounts.GetAccount(config.From)
		if err != nil {
			return err
		}
		isOwner := false
		for _, owner := range owners {
			if strings.EqualFold(owner, fromAcc.Address) {
				isOwner = true
				break
			}
		}
		if !isOwner {
			return fmt.Errorf(
				"%s is not an owner of Safe %s",
				fromAcc.Address, tc.To,
			)
		}
	}
	tc.FromAcc = fromAcc
	tc.From = fromAcc.Address

	r := tc.Reader
	if config.GasPrice == 0 {
		tc.GasPrice, err = r.RecommendedGasPrice()
		if err != nil {
			showNodeErrorGuidance(u, config.Network())
			return fmt.Errorf("getting recommended gas price failed: %w", err)
		}
	} else {
		tc.GasPrice = config.GasPrice
	}

	if config.Nonce == 0 {
		tc.Nonce, err = r.GetMinedNonce(tc.From)
		if err != nil {
			showNodeErrorGuidance(u, config.Network())
			return fmt.Errorf("getting nonce failed: %w", err)
		}
	} else {
		tc.Nonce = config.Nonce
	}

	tc.TxType, err = ValidTxType(r, config.Network())
	if err != nil {
		showNodeErrorGuidance(u, config.Network())
		return fmt.Errorf("couldn't determine proper tx type: %w", err)
	}
	if tc.TxType == types.LegacyTxType && config.TipGas > 0 {
		return fmt.Errorf("we are doing legacy tx hence we ignore tip gas parameter")
	}
	if tc.TxType == types.DynamicFeeTxType {
		if config.TipGas == 0 {
			tc.TipGas, err = r.GetSuggestedGasTipCap()
			if err != nil {
				showNodeErrorGuidance(u, config.Network())
				return fmt.Errorf("couldn't estimate recommended gas price: %w", err)
			}
		} else {
			tc.TipGas = config.TipGas
		}
	}

	bc, err := util.EthBroadcaster(config.Network())
	if err != nil {
		showNodeErrorGuidance(u, config.Network())
		return fmt.Errorf("couldn't connect to broadcaster: %w", err)
	}
	tc.Broadcaster = bc

	collector, err := safe.NewTxServiceCollector(config.Network().GetChainID())
	if err != nil {
		return fmt.Errorf(
			"couldn't init safe transaction service client for chain %d: %w",
			config.Network().GetChainID(), err,
		)
	}
	tc.Collector = collector

	cmd.SetContext(WithTxContext(cmd.Context(), tc))
	return nil
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
	if err := config.SetNetwork(config.NetworkString); err != nil {
		return err
	}

	addr, _, err := DefaultABIResolver{}.GetAddressFromString(args[0])
	if err != nil {
		addr = args[0]
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
	if err := config.SetNetwork(config.NetworkString); err != nil {
		return err
	}

	addr, _, err := DefaultABIResolver{}.GetAddressFromString(args[0])
	if err != nil {
		addr = args[0]
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
