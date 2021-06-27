package kyberdao

import (
	"fmt"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

var withdrawCmd = &cobra.Command{
	Use:              "withdraw",
	Short:            "Withdraw your KNC from KyberDAO",
	TraverseChildren: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		// process from to get address
		acc, err := accounts.GetAccount(config.From)
		if err != nil {
			return fmt.Errorf("Couldn't interpret addresss. Please double check your -f flag. %w\n", err)
		} else {
			config.FromAcc = acc
			config.From = acc.Address
		}

		return Preprocess(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		PrintENV()

		reader, err := util.EthReader(config.Network())
		if err != nil {
			cmd.Printf("Couldn't init eth reader: %s\n", err)
			return
		}

		dao := NewKyberDAO(reader, StakingContract, DaoContract, FeeHandler)
		stakeInfo, err := dao.AllStakeRelatedInfo(config.From, Epoch)
		if err != nil {
			cmd.Printf("Couldn't get stake information: %s\n", err)
			return
		}
		PrintStakeInformation(cmd, stakeInfo)

		if stakeInfo.FutureStake.Cmp(big.NewInt(0)) == 0 {
			cmd.Printf("You don't have any available KNC to withdraw. Abort.\n")
			return
		}

		rootCmd := cmd.Root()
		txCmd, txArgs, err := rootCmd.Find([]string{
			"contract",
			"tx",
			StakingContract,
		})
		if err != nil {
			cmd.Printf("Couldn't find tx command: %s\n", err)
		}
		config.MethodIndex = 5
		config.GasLimit = 500000
		config.PrefillStr = ""
		txCmd.PersistentPreRunE(txCmd, txArgs)
		txCmd.Run(txCmd, txArgs)
	},
}

func init() {
	withdrawCmd.PersistentFlags().Float64VarP(&config.GasPrice, "gasprice", "p", 0, "Gas price in gwei. If default value is used, we will use https://ethgasstation.info/ to get fast gas price. The gas price to be used in the tx is gas price + extra gas price")
	withdrawCmd.PersistentFlags().Float64VarP(&config.ExtraGasPrice, "extraprice", "P", 0, "Extra gas price in gwei. The gas price to be used in the tx is gas price + extra gas price")
	withdrawCmd.PersistentFlags().Uint64VarP(&config.Nonce, "nonce", "n", 0, "Nonce of the from account. If default value is used, we will use the next available nonce of from account")
	withdrawCmd.PersistentFlags().StringVarP(&config.From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
	withdrawCmd.PersistentFlags().BoolVarP(&config.DontBroadcast, "dry", "d", false, "Will not broadcast the tx, only show signed tx.")
	withdrawCmd.PersistentFlags().BoolVarP(&config.DontWaitToBeMined, "no-wait", "F", false, "Will not wait the tx to be mined.")
	withdrawCmd.MarkFlagRequired("from")
}
