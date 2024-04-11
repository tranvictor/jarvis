package util

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/accounts/types"
	. "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/config/state"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/util"
)

// CommonFunctionCallPreprocess processes args passed to the command in order to
// initiate config's variables in a conventional way across many Function Call alike
// commands.
func CommonFunctionCallPreprocess(cmd *cobra.Command, args []string) (err error) {
	config.PrefillStr = strings.Trim(config.PrefillStr, " ")
	if config.PrefillStr != "" {
		config.PrefillMode = true
		config.PrefillParams = strings.Split(config.PrefillStr, "|")
		for i := range config.PrefillParams {
			config.PrefillParams[i] = strings.Trim(config.PrefillParams[i], " ")
		}
	}
	PrintElapseTime(Start, "after processing prefill string")

	config.Value, err = FloatStringToBig(config.RawValue, 18)
	if err != nil {
		return fmt.Errorf("couldn't parse -v param: %s", err)
	}

	if config.Value.Cmp(big.NewInt(0)) < 0 {
		return fmt.Errorf("-v param can't be negative")
	}

	PrintElapseTime(Start, "after processing config.Value")

	if len(args) == 0 {
		config.To = "" // this is to indicate a contract creation tx
	} else {
		config.To, _, err = util.GetAddressFromString(args[0])
		if err != nil {
			nwks, txs := ScanForTxs(args[0])
			if len(txs) == 0 {
				return fmt.Errorf("can't interpret the contract address")
			}
			config.Tx = txs[0]
			if nwks[0] != "" {
				config.SetNetwork(nwks[0])
			}

			reader, err := util.EthReader(config.Network())
			if err != nil {
				return fmt.Errorf("couldn't connect to blockchain\n")
			}

			PrintElapseTime(Start, "after initiating tx hash and ethreader, begin to get txinfo from hash")

			txinfo, err := reader.TxInfoFromHash(config.Tx)
			if err != nil {
				return fmt.Errorf("couldn't get tx info from the blockchain: %s\n", err)
			}
			state.TxInfo = &txinfo
			config.To = state.TxInfo.Tx.To().Hex()
		}
	}

	PrintElapseTime(Start, "after processing config.To & config.TxInfo")

	return nil
}

// CommonTxPreprocess processes args passed to the command in order to
// initiate config's variables in a conventional way across many commands
// that do txs.
func CommonTxPreprocess(cmd *cobra.Command, args []string) (err error) {
	err = CommonFunctionCallPreprocess(cmd, args)
	if err != nil {
		return err
	}

	a, err := util.GetABI(config.To, config.Network())
	if err != nil {
		if config.ForceERC20ABI {
			a = GetERC20ABI()
		} else if config.CustomABI != "" {
			a, err = util.ReadCustomABI(config.To, config.CustomABI, config.Network())
			if err != nil {
				return fmt.Errorf("reading cusom abi failed: %w", err)
			}
		}
	}

	PrintElapseTime(Start, "after getting abi")

	// loosely check by checking a set of method names

	isGnosisMultisig := false

	if err == nil {
		isGnosisMultisig, err = util.IsGnosisMultisig(a)
		if err != nil {
			return fmt.Errorf("checking if the address is gnosis multisig classic failed: %w", err)
		}
	}

	PrintElapseTime(Start, "after checking if address is a msig")

	if config.From == "" && isGnosisMultisig {
		multisigContract, err := msig.NewMultisigContract(
			config.To,
			config.Network(),
		)
		if err != nil {
			return err
		}
		owners, err := multisigContract.Owners()
		if err != nil {
			return fmt.Errorf("getting msig owners failed: %w", err)
		}

		PrintElapseTime(Start, "after getting msig owner list")

		var acc types.AccDesc
		count := 0
		for _, owner := range owners {
			a, err := accounts.GetAccount(owner)
			if err == nil {
				acc = a
				count++
			}
		}
		if count == 0 {
			return fmt.Errorf(
				"You don't have any wallet which is this multisig signer. Please jarvis wallet add to add the wallet.",
			)
		}
		if count != 1 {
			return fmt.Errorf(
				"You have many wallets that are this multisig signers. Please specify only 1.",
			)
		}
		config.FromAcc = acc
		config.From = acc.Address

		PrintElapseTime(Start, "after getting config.From config.FromAcc")

	} else {
		// process from to get address
		acc, err := accounts.GetAccount(config.From)
		if err != nil {
			return err
		} else {
			config.FromAcc = acc
			config.From = acc.Address
		}

		PrintElapseTime(Start, "after getting config.From config.FromAcc")

	}

	reader, err := util.EthReader(config.Network())
	if err != nil {
		return err
	}

	// config.GasPrice, err = util.FloatStringToBig(config.RawGasPrice, 9)
	// if err != nil {
	// 	return fmt.Errorf("couldn't parse gas price param: %s", err)
	// }

	if config.GasPrice == 0 {
		config.GasPrice, err = reader.RecommendedGasPrice()
		if err != nil {
			return fmt.Errorf("getting recommended gas price failed: %w", err)
		}
	}

	PrintElapseTime(Start, "after getting gas price")

	// var Nonce uint64
	if config.Nonce == 0 {
		config.Nonce, err = reader.GetMinedNonce(config.From)
		if err != nil {
			return fmt.Errorf("getting nonce failed: %w", err)
		}
	}

	isDynamicFeeAvailable, err := reader.CheckDynamicFeeTxAvailable()
	if err != nil {
		return fmt.Errorf("checking if chain has dynamic fee feature failed: %w", err)
	}

	if isDynamicFeeAvailable {
		config.TipGas, err = reader.GetSuggestedGasTipCap()
		if err != nil {
			return fmt.Errorf("getting tip gas failed: %w", err)
		}
	}

	return nil
}
