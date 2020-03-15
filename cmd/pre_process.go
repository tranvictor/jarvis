package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/util"
)

func CommonFunctionCallPreprocess(cmd *cobra.Command, args []string) (err error) {
	config.PrefillStr = strings.Trim(config.PrefillStr, " ")
	if config.PrefillStr != "" {
		config.PrefillMode = true
		config.PrefillParams = strings.Split(config.PrefillStr, "|")
		for i, _ := range config.PrefillParams {
			config.PrefillParams[i] = strings.Trim(config.PrefillParams[i], " ")
		}
	}

	if config.Value < 0 {
		return fmt.Errorf("value can't be negative")
	}

	config.To, _, err = util.GetAddressFromString(args[0])
	if err != nil {
		txs := util.ScanForTxs(args[0])
		if len(txs) == 0 {
			return fmt.Errorf("can't interpret the contract address")
		} else {
			config.Tx = txs[0]

			reader, err := util.EthReader(config.Network)
			if err != nil {
				return fmt.Errorf("Couldn't connect to blockchain.\n")
			}

			txinfo, err := reader.TxInfoFromHash(config.Tx)
			if err != nil {
				return fmt.Errorf("Couldn't get tx info from the blockchain: %s\n", err)
			}
			config.TxInfo = &txinfo
			config.To = config.TxInfo.Tx.To().Hex()
		}
	}

	fmt.Printf("Interpreted to address: %s\n", util.VerboseAddress(config.To))
	return nil
}

func CommonTxPreprocess(cmd *cobra.Command, args []string) (err error) {
	err = CommonFunctionCallPreprocess(cmd, args)
	if err != nil {
		return err
	}

	isGnosisMultisig, err := util.IsGnosisMultisig(config.To, config.Network)
	if err != nil {
		return err
	}

	if config.From == "" && isGnosisMultisig {
		multisigContract, err := msig.NewMultisigContract(
			config.To,
			config.Network,
		)
		if err != nil {
			return err
		}
		owners, err := multisigContract.Owners()
		if err != nil {
			return err
		}

		var acc accounts.AccDesc
		count := 0
		for _, owner := range owners {
			a, err := accounts.GetAccount(owner)
			if err == nil {
				acc = a
				count++
			}
		}
		if count == 0 {
			return fmt.Errorf("You don't have any wallet which is this multisig signer. Please jarvis wallet add to add the wallet.")
		}
		if count != 1 {
			return fmt.Errorf("You have many wallets that are this multisig signers. Please specify only 1.")
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

	fmt.Printf("Network: %s\n", config.Network)
	reader, err := util.EthReader(config.Network)
	if err != nil {
		return err
	}

	// var GasPrice float64
	if config.GasPrice == 0 {
		config.GasPrice, err = reader.RecommendedGasPrice()
		if err != nil {
			return err
		}
	}

	// var Nonce uint64
	if config.Nonce == 0 {
		config.Nonce, err = reader.GetMinedNonce(config.From)
		if err != nil {
			return err
		}
	}
	return nil
}
