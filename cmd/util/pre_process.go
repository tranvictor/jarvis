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
	"github.com/tranvictor/jarvis/config/state"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/util"
)

// CommonFunctionCallPreprocess processes args passed to the command in order to
// initiate config's variables in a conventional way across many Function Call alike
// commands.
func CommonFunctionCallPreprocess(cmd *cobra.Command, args []string) (err error) {
	if err = config.SetNetwork(config.NetworkString); err != nil {
		return err
	}

	config.PrefillStr = strings.Trim(config.PrefillStr, " ")
	if config.PrefillStr != "" {
		config.PrefillMode = true
		config.PrefillParams = strings.Split(config.PrefillStr, "|")
		for i := range config.PrefillParams {
			config.PrefillParams[i] = strings.Trim(config.PrefillParams[i], " ")
		}
	}

	config.Value, err = jarviscommon.FloatStringToBig(config.RawValue, 18)
	if err != nil {
		return fmt.Errorf("couldn't parse -v param: %s", err)
	}

	if config.Value.Cmp(big.NewInt(0)) < 0 {
		return fmt.Errorf("-v param can't be negative")
	}

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
				err = config.SetNetwork(nwks[0])
				if err != nil {
					return err
				}
			}

			reader, err := util.EthReader(config.Network())
			if err != nil {
				return fmt.Errorf("couldn't connect to blockchain: %w", err)
			}

			txinfo, err := reader.TxInfoFromHash(config.Tx)
			if err != nil {
				return fmt.Errorf("couldn't get tx info from the blockchain: %w", err)
			}
			state.TxInfo = &txinfo
			config.To = state.TxInfo.Tx.To().Hex()
		}
	}

	return nil
}

// CommonTxPreprocess processes args passed to the command in order to
// initiate config's variables in a conventional way across many commands
// that do txs.
func CommonTxPreprocess(cmd *cobra.Command, args []string) (err error) {
	if err = config.SetNetwork(config.NetworkString); err != nil {
		return err
	}

	err = CommonFunctionCallPreprocess(cmd, args)
	if err != nil {
		return err
	}

	a, err := util.GetABI(config.To, config.Network())
	if err != nil {
		if config.ForceERC20ABI {
			a = jarviscommon.GetERC20ABI()
		} else if config.CustomABI != "" {
			a, err = util.ReadCustomABI(config.To, config.CustomABI, config.Network())
			if err != nil {
				return fmt.Errorf("reading custom abi failed: %w", err)
			}
		}
	}

	// loosely check by checking a set of method names

	isGnosisMultisig := false

	if err == nil {
		isGnosisMultisig, err = util.IsGnosisMultisig(a)
		if err != nil {
			return fmt.Errorf("checking if the address is gnosis multisig classic failed: %w", err)
		}
	}

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

		var acc jtypes.AccDesc
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
				"you don't have any wallet which is this multisig signer. please jarvis wallet add to add the wallet",
			)
		}
		if count != 1 {
			return fmt.Errorf(
				"you have many wallets that are this multisig signers. please specify only 1",
			)
		}
		config.FromAcc = acc
		config.From = acc.Address

	} else {
		// process from to get address
		acc, err := accounts.GetAccount(config.From)
		if err != nil {
			return err
		} else {
			config.FromAcc = acc
			config.From = acc.Address
		}

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

	// var Nonce uint64
	if config.Nonce == 0 {
		config.Nonce, err = reader.GetMinedNonce(config.From)
		if err != nil {
			return fmt.Errorf("getting nonce failed: %w", err)
		}
	}

	config.TxType, err = ValidTxType(reader, config.Network())
	if err != nil {
		return fmt.Errorf("couldn't determine proper tx type: %w", err)
	}

	if config.TxType == types.LegacyTxType && config.TipGas > 0 {
		return fmt.Errorf("we are doing legacy tx hence we ignore tip gas parameter")
	}

	if config.TxType == types.DynamicFeeTxType {
		if config.TipGas == 0 {
			config.TipGas, err = reader.GetSuggestedGasTipCap()
			if err != nil {
				return fmt.Errorf("couldn't estimate recommended gas price: %w", err)
			}
		}
	}

	return nil
}
