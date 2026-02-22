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
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
)

// CommonFunctionCallPreprocess populates a TxContext with the values derivable
// from the function-call arguments: target address, parsed value, prefill params,
// and an optional TxInfo when the argument is a tx hash. It attaches the context
// to the cobra command so Run functions can retrieve it via TxContextFrom.
func CommonFunctionCallPreprocess(u ui.UI, cmd *cobra.Command, args []string) (err error) {
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
			nwks, txs := ScanForTxs(args[0])
			if len(txs) == 0 {
				return fmt.Errorf("can't interpret the contract address")
			}
			config.Tx = txs[0]
			if nwks[0] != "" {
				if err = config.SetNetwork(nwks[0]); err != nil {
					return err
				}
			}

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
			return fmt.Errorf("getting recommended gas price failed: %w", err)
		}
	} else {
		tc.GasPrice = config.GasPrice
	}

	if config.Nonce == 0 {
		tc.Nonce, err = reader.GetMinedNonce(tc.From)
		if err != nil {
			return fmt.Errorf("getting nonce failed: %w", err)
		}
	} else {
		tc.Nonce = config.Nonce
	}

	tc.TxType, err = ValidTxType(reader, config.Network())
	if err != nil {
		return fmt.Errorf("couldn't determine proper tx type: %w", err)
	}

	if tc.TxType == types.LegacyTxType && config.TipGas > 0 {
		return fmt.Errorf("we are doing legacy tx hence we ignore tip gas parameter")
	}

	if tc.TxType == types.DynamicFeeTxType {
		if config.TipGas == 0 {
			tc.TipGas, err = reader.GetSuggestedGasTipCap()
			if err != nil {
				return fmt.Errorf("couldn't estimate recommended gas price: %w", err)
			}
		} else {
			tc.TipGas = config.TipGas
		}
	}

	bc, err := util.EthBroadcaster(config.Network())
	if err != nil {
		return fmt.Errorf("couldn't connect to broadcaster: %w", err)
	}
	tc.Broadcaster = bc

	cmd.SetContext(WithTxContext(cmd.Context(), tc))
	return nil
}
