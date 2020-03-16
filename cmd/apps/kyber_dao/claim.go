package kyberdao

import (
	"fmt"
	"math/big"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

var claimCmd = &cobra.Command{
	Use:               "claim-reward",
	Short:             "Claim your reward from KyberDAO",
	TraverseChildren:  true,
	PersistentPreRunE: Preprocess,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			cmd.Printf("Please specify epoch number as a param. Abort.\n")
			return
		}
		var err error
		Epoch, err = strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			cmd.Printf("Couldn't convert %s to number. Abort.\n", args[0])
			return
		}

		PrintENV()

		reader, err := util.EthReader(config.Network)
		if err != nil {
			cmd.Printf("Couldn't init eth reader: %s\n", err)
			return
		}

		dao := NewKyberDAO(reader, StakingContract, DaoContract, FeeHandler)

		currentEpoch, err := dao.CurrentEpoch()
		if err != nil {
			cmd.Printf("Couldn't get current epoch: %s\n", err)
			return
		}

		if currentEpoch <= Epoch {
			cmd.Printf("You cannot claim rewards for an epoch in the future.\n")
			return
		}

		reward, totalReward, share, isClaimed, err := dao.GetRewardInfo(config.From, Epoch)
		if err != nil {
			cmd.Printf("Couldn't get reward information of %s at epoch %d: %s\n", config.From, Epoch, err)
			return
		}
		cmd.Printf("\nYour reward information for epoch %d:\n", Epoch)
		if isClaimed {
			cmd.Printf("%f ETH - %f%% of total reward pool (%f ETH) | CLAIMED\n", ethutils.BigToFloat(reward, 18), share, ethutils.BigToFloat(totalReward, 18))
		} else {
			cmd.Printf("%f ETH - %f%% of total reward pool (%f ETH)\n", ethutils.BigToFloat(reward, 18), share, ethutils.BigToFloat(totalReward, 18))
		}
		cmd.Printf("\n")

		if isClaimed {
			cmd.Printf("You already claimed your reward at this epoch. Cannot claim more than once for an epoch.\n")
			return
		}

		if reward.Cmp(big.NewInt(0)) == 0 {
			cmd.Printf("You don't have any reward at this epoch to claim. Abort.\n")
			return
		}

		rootCmd := cmd.Root()
		txCmd, txArgs, err := rootCmd.Find([]string{
			"contract",
			"tx",
			DaoContract,
		})
		if err != nil {
			cmd.Printf("Couldn't find tx command: %s\n", err)
		}
		config.MethodIndex = 3
		config.PrefillStr = fmt.Sprintf("%s|%d", config.From, Epoch)
		txCmd.PersistentPreRunE(txCmd, txArgs)
		txCmd.Run(txCmd, txArgs)
	},
}

func init() {
	claimCmd.PersistentFlags().Float64VarP(&config.GasPrice, "gasprice", "p", 0, "Gas price in gwei. If default value is used, we will use https://ethgasstation.info/ to get fast gas price. The gas price to be used in the tx is gas price + extra gas price")
	claimCmd.PersistentFlags().Float64VarP(&config.ExtraGasPrice, "extraprice", "P", 0, "Extra gas price in gwei. The gas price to be used in the tx is gas price + extra gas price")
	claimCmd.PersistentFlags().Uint64VarP(&config.Nonce, "nonce", "n", 0, "Nonce of the from account. If default value is used, we will use the next available nonce of from account")
	claimCmd.PersistentFlags().StringVarP(&config.From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
	claimCmd.PersistentFlags().BoolVarP(&config.DontBroadcast, "dry", "d", false, "Will not broadcast the tx, only show signed tx.")
	claimCmd.PersistentFlags().BoolVarP(&config.DontWaitToBeMined, "no-wait", "F", false, "Will not wait the tx to be mined.")
	claimCmd.MarkFlagRequired("from")
}
