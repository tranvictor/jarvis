package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/db"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/util"
)

func CommonTxPreprocess(cmd *cobra.Command, args []string) (err error) {
	PrefillStr = strings.Trim(PrefillStr, " ")
	if PrefillStr != "" {
		PrefillMode = true
		PrefillParams = strings.Split(PrefillStr, "|")
		for i, _ := range PrefillParams {
			PrefillParams[i] = strings.Trim(PrefillParams[i], " ")
		}
	}

	if Value < 0 {
		return fmt.Errorf("value can't be negative")
	}

	To, _, err = getAddressFromString(args[0])
	if err != nil {
		txs := util.ScanForTxs(args[0])
		if len(txs) == 0 {
			return fmt.Errorf("can't interpret the contract address")
		} else {
			Tx = txs[0]

			reader, err := util.EthReader(Network)
			if err != nil {
				return fmt.Errorf("Couldn't connect to blockchain.\n")
			}

			txinfo, err := reader.TxInfoFromHash(Tx)
			if err != nil {
				return fmt.Errorf("Couldn't get tx info from the blockchain: %s\n", err)
			}
			TxInfo = &txinfo
			To = TxInfo.Tx.To().Hex()
		}
	}

	isGnosisMultisig, err := util.IsGnosisMultisig(To, Network)
	if err != nil {
		return err
	}

	if From == "" && isGnosisMultisig {
		multisigContract, err := msig.NewMultisigContract(
			To,
			Network,
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
		FromAcc = acc
		From = acc.Address
	} else {
		// process from to get address
		acc, err := accounts.GetAccount(From)
		if err != nil {
			return err
		} else {
			FromAcc = acc
			From = acc.Address
		}
	}

	fmt.Printf("Network: %s\n", Network)
	reader, err := util.EthReader(Network)
	if err != nil {
		return err
	}

	// var GasPrice float64
	if GasPrice == 0 {
		GasPrice, err = reader.RecommendedGasPrice()
		if err != nil {
			return err
		}
	}

	// var Nonce uint64
	if Nonce == 0 {
		Nonce, err = reader.GetMinedNonce(From)
		if err != nil {
			return err
		}
	}
	return nil
}

func getAddressFromString(str string) (addr string, name string, err error) {
	addrDesc, err := db.GetAddress(str)
	if err != nil {
		name = "Unknown"
		addresses := util.ScanForAddresses(str)
		if len(addresses) == 0 {
			return "", "", fmt.Errorf("address not found for \"%s\"", str)
		}
		addr = addresses[0]
	} else {
		name = addrDesc.Desc
		addr = addrDesc.Address
	}
	return addr, name, nil
}
