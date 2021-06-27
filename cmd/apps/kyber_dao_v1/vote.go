package kyberdao

import (
	"fmt"
	"math/big"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

var voteCmd = &cobra.Command{
	Use:              "vote",
	Short:            "Vote on a KyberDAO voting campaign",
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
		if len(args) < 1 {
			cmd.Printf("Please specify campaign ID number as a param. Abort.\n")
			return
		}
		var err error
		CampaignID, err = strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			cmd.Printf("Couldn't convert %s to number. Abort.\n", args[0])
			return
		}

		PrintENV()

		reader, err := util.EthReader(config.Network())
		if err != nil {
			cmd.Printf("Couldn't init eth reader: %s\n", err)
			return
		}

		dao := NewKyberDAO(reader, StakingContract, DaoContract, FeeHandler)
		campaignRelatedInfo, err := dao.AllCampaignRelatedInfo(config.From, big.NewInt(int64(CampaignID)))
		if err != nil {
			cmd.Printf("Couldn't get campaign information: %s\n", err)
			return
		}

		PrintCampaignInformation(cmd, campaignRelatedInfo)

		rootCmd := cmd.Root()
		txCmd, txArgs, err := rootCmd.Find([]string{
			"contract",
			"tx",
			DaoContract,
		})
		if err != nil {
			cmd.Printf("Couldn't find tx command: %s\n", err)
		}
		config.MethodIndex = 10
		config.PrefillStr = fmt.Sprintf("%d|?", CampaignID)
		txCmd.PersistentPreRunE(txCmd, txArgs)
		txCmd.Run(txCmd, txArgs)
	},
}

func init() {
	voteCmd.PersistentFlags().Float64VarP(&config.GasPrice, "gasprice", "p", 0, "Gas price in gwei. If default value is used, we will use https://ethgasstation.info/ to get fast gas price. The gas price to be used in the tx is gas price + extra gas price")
	voteCmd.PersistentFlags().Float64VarP(&config.ExtraGasPrice, "extraprice", "P", 0, "Extra gas price in gwei. The gas price to be used in the tx is gas price + extra gas price")
	voteCmd.PersistentFlags().Uint64VarP(&config.Nonce, "nonce", "n", 0, "Nonce of the from account. If default value is used, we will use the next available nonce of from account")
	voteCmd.PersistentFlags().StringVarP(&config.From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
	voteCmd.PersistentFlags().BoolVarP(&config.DontBroadcast, "dry", "d", false, "Will not broadcast the tx, only show signed tx.")
	voteCmd.PersistentFlags().BoolVarP(&config.DontWaitToBeMined, "no-wait", "F", false, "Will not wait the tx to be mined.")
}
