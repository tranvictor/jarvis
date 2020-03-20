package kyberdao

import (
	"fmt"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

var createCamCmd = &cobra.Command{
	Use:               "create-campaign",
	Short:             "Create voting campaign for KyberDAO, this feature is used by Kyber team only and will initiate a multisig transaction",
	TraverseChildren:  true,
	PersistentPreRunE: Preprocess,
	Run: func(cmd *cobra.Command, args []string) {
		PrintENV()
		reader, err := util.EthReader(config.Network)
		if err != nil {
			cmd.Printf("Couldn't init eth reader: %s\n", err)
			return
		}

		dao := NewKyberDAO(reader, StakingContract, DaoContract, FeeHandler)

		timeInfo, err := dao.AllTimeRelatedInfo()
		if err != nil {
			cmd.Printf("Couldn't get information about time: %s\n", err)
			return
		}

		cmd.Printf("Current block: %d\n", timeInfo.CurrentBlock)
		cmd.Printf("Current epoch: %d\n", timeInfo.CurrentEpoch)
		cmd.Printf("Next epoch (%d -> %d) starts in: %s (%d blocks)\n",
			timeInfo.NextEpochStartBlock,
			timeInfo.NextEpochEndBlock,
			timeInfo.TimeUntilNextEpoch,
			big.NewInt(0).Sub(timeInfo.NextEpochStartBlock, timeInfo.CurrentBlock),
		)

		typeOptions := []string{
			"general", "fee", "brr",
		}

		cType := util.PromptItemInList(
			fmt.Sprintf("Enter campaign type (in %v)", typeOptions),
			typeOptions,
		)

		var CampType uint8
		var epochOfTheCam string

		switch cType {
		case "general":
			CampType = 0
		case "fee":
			CampType = 1
		case "brr":
			CampType = 2
		}

		startBlockBig := util.PromptNumber(
			"Enter start block for the campaign",
			func(number *big.Int) error {
				if number.Cmp(timeInfo.CurrentBlock) <= 0 {
					return fmt.Errorf("Can't create a campaign with start block in the past")
				}

				if number.Cmp(timeInfo.NextEpochEndBlock) >= 0 {
					return fmt.Errorf("Can't create a campaign too far in the future (can only create campaigns for current and next epoch)")
				}

				return nil
			},
			config.Network,
		)
		if startBlockBig.Cmp(timeInfo.NextEpochStartBlock) < 0 {
			epochOfTheCam = "current"
		} else {
			epochOfTheCam = "next"
		}

		endBlockBig := util.PromptNumber(
			"Enter end block for the campaign",
			func(number *big.Int) error {
				if big.NewInt(0).Sub(number, startBlockBig).Cmp(big.NewInt(int64(MinCamDuration))) < 0 {
					return fmt.Errorf("Can't create a campaign that lasts less than %d blocks", MinCamDuration)
				}

				if epochOfTheCam == "current" && number.Cmp(timeInfo.NextEpochStartBlock) >= 0 {
					return fmt.Errorf("Can't create a campaign that lasts across 2 epochs")
				}

				if epochOfTheCam == "next" && number.Cmp(timeInfo.NextEpochEndBlock) > 0 {
					return fmt.Errorf("Can't create a campaign that lasts across 2 epochs")
				}
				return nil
			},
			config.Network,
		)
		// options
		var Options []*big.Int

		noOfOptions := util.PromptNumber(
			"Enter number of options (1-8)",
			func(number *big.Int) error {
				n := number.Int64()
				if n < 1 || n > 8 {
					return fmt.Errorf("Number of options must be in [1, 8]")
				}
				return nil
			},
			config.Network,
		)

		switch cType {
		case "general":
			for i := int64(1); i <= noOfOptions.Int64(); i++ {
				Options = append(Options, big.NewInt(i))
			}
		case "fee":
			for i := int64(1); i <= noOfOptions.Int64(); i++ {
				fee := util.PromptPercentageBps(
					fmt.Sprintf("Enter fee for option #%d", i),
					10000,
					config.Network,
				)
				Options = append(Options, fee)
			}
		case "brr":
			for i := int64(1); i <= noOfOptions.Int64(); i++ {
				fee := PromptBRROption(
					fmt.Sprintf("Enter fee (format: rebateBps, rewardBps) for option #%d", i),
				)
				Options = append(Options, fee)
			}
		}
		// formula params
		// link
		Link := util.PromptInput("Enter ref link for the campaign")

		rootCmd := cmd.Root()
		txCmd, txArgs, err := rootCmd.Find([]string{
			"msig",
			"init",
			CampaignCreator,
		})
		if err != nil {
			cmd.Printf("Couldn't find tx command: %s\n", err)
		}
		config.MsigTo = DaoContract
		config.MethodIndex = 7
		config.ExtraGasLimit = 500000
		config.PrefillStr = fmt.Sprintf(
			// campType|startBlock|endBlock|min|c|t|options|link
			"%d|%d|%d|%d|%d|%d|%s|\"%s\"",
			CampType,
			startBlockBig,
			endBlockBig,
			big.NewInt(0),
			big.NewInt(0),
			big.NewInt(0),
			optionStr(Options),
			Link,
		)
		// cmd.Printf("Prefill string: %s\n", config.PrefillStr)
		txCmd.PersistentPreRunE(txCmd, txArgs)
		txCmd.Run(txCmd, txArgs)
	},
}

func init() {
	createCamCmd.PersistentFlags().Float64VarP(&config.GasPrice, "gasprice", "p", 0, "Gas price in gwei. If default value is used, we will use https://ethgasstation.info/ to get fast gas price. The gas price to be used in the tx is gas price + extra gas price")
	createCamCmd.PersistentFlags().Float64VarP(&config.ExtraGasPrice, "extraprice", "P", 0, "Extra gas price in gwei. The gas price to be used in the tx is gas price + extra gas price")
	createCamCmd.PersistentFlags().Uint64VarP(&config.Nonce, "nonce", "n", 0, "Nonce of the from account. If default value is used, we will use the next available nonce of from account")
	createCamCmd.PersistentFlags().BoolVarP(&config.DontBroadcast, "dry", "d", false, "Will not broadcast the tx, only show signed tx.")
	createCamCmd.PersistentFlags().BoolVarP(&config.DontWaitToBeMined, "no-wait", "F", false, "Will not wait the tx to be mined.")
	createCamCmd.PersistentFlags().Uint64VarP(&Epoch, "epoch", "e", 0, "Epoch to read staking and dao data.")
}
