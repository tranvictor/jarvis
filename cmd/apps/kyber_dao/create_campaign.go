package kyberdao

import (
	"fmt"
	"math/big"
	"time"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

var KIP2 bool

func adjustKIP2BRR(rebate, reward, burn uint64, increment string) (newRebate, newReward, newBurn uint64) {
	switch increment {
	case "rebate":
		if rebate > 9500 {
			return 10000, 0, 0
		}
		newRebate = rebate + 500
		if reward < 500*uint64(float64(reward)/float64(reward+burn)) {
			return newRebate, 0, 10000 - newRebate
		}
		newReward = reward - uint64(500*(float64(reward)/float64(reward+burn)))
		newBurn = 10000 - newRebate - newReward
		return newRebate, newReward, newBurn
	case "reward":
		if reward > 9500 {
			return 0, 10000, 0
		}
		newReward = reward + 500
		if rebate < 500*uint64(float64(rebate)/float64(rebate+burn)) {
			return 0, newReward, 10000 - newReward
		}
		newRebate = rebate - uint64(500*(float64(rebate)/float64(rebate+burn)))
		newBurn = 10000 - newRebate - newReward
		return newRebate, newReward, newBurn
	case "burn":
		if burn > 9500 {
			return 0, 0, 10000
		}
		newBurn = burn + 500
		if rebate < 500*uint64(float64(rebate)/float64(rebate+reward)) {
			return 0, 10000 - newBurn, newBurn
		}
		newRebate = rebate - uint64(500*(float64(rebate)/float64(rebate+reward)))
		newReward = 10000 - newRebate - newBurn
		return newRebate, newReward, newBurn
	}
	panic("unsupport kip2 increment")
}

func adjustBRR(rebate, reward, burn uint64, increment string, kip string) (newRebate, newReward, newBurn uint64) {
	switch kip {
	case "kip2":
		return adjustKIP2BRR(rebate, reward, burn, increment)
	default:
		panic(fmt.Sprintf("%s is not supported", kip))
	}
}

func generateBRROptions(dao *KyberDAO, epoch uint64) (options []*big.Int, err error) {
	camIDs, err := dao.GetCampaignIDs(epoch)
	if err != nil {
		return nil, err
	}

	for _, id := range camIDs {
		cinfo, err := dao.AllCampaignRelatedInfo(config.From, id)
		if err != nil {
			return nil, err
		}
		if cinfo.Campaign.Type() == "brr" {
			if uint64(time.Now().Unix()) < cinfo.Campaign.EndTimestamp.Uint64() {
				return nil, fmt.Errorf("BRR campaign is not finished yet. Aborted.")
			}

			if !cinfo.Campaign.HasWinningOption() {
				return nil, fmt.Errorf("BRR campaign doesn't have any winning options. Aborted.")
			}

			winningID := cinfo.Campaign.WinningOption.Int64()
			fmt.Printf("winning id: %d\n", winningID)
			winningOption := cinfo.Campaign.Options[winningID-1]

			rebateBig := big.NewInt(0).Rsh(winningOption, 128)
			rebate := rebateBig.Uint64()

			temp := big.NewInt(0).Lsh(rebateBig, 128)
			rewardBig := big.NewInt(0).Sub(winningOption, temp)
			reward := rewardBig.Uint64()

			burn := 10000 - rebate - reward

			options = []*big.Int{EncodeBRROption(rebate, reward)}
			fmt.Printf("Current BRR setting    : Rebate(%d) Reward(%d) Burn(%d)\n", rebate, reward, burn)

			rebate1, reward1, burn1 := adjustBRR(rebate, reward, burn, "burn", "kip2")
			options = append(options, EncodeBRROption(rebate1, reward1))
			fmt.Printf("Increase Burn setting  : Rebate(%d) Reward(%d) Burn(%d)\n", rebate1, reward1, burn1)

			rebate2, reward2, burn2 := adjustBRR(rebate, reward, burn, "reward", "kip2")
			options = append(options, EncodeBRROption(rebate2, reward2))
			fmt.Printf("Increase Reward setting: Rebate(%d) Reward(%d) Burn(%d)\n", rebate2, reward2, burn2)

			rebate3, reward3, burn3 := adjustBRR(rebate, reward, burn, "rebate", "kip2")
			options = append(options, EncodeBRROption(rebate3, reward3))
			fmt.Printf("Increase Rebate setting: Rebate(%d) Reward(%d) Burn(%d)\n", rebate3, reward3, burn3)

			return options, nil
		}
	}
	return nil, fmt.Errorf("Couldn't find BRR campaign in this epoch. Aborted.")
}

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
		cmd.Printf("Next epoch (%d -> %d) starts in: %s\n",
			timeInfo.NextEpochStartTimestamp,
			timeInfo.NextEpochEndTimestamp,
			timeInfo.TimeUntilNextEpoch,
		)

		typeOptions := []string{
			"general", "fee", "brr",
		}

		cType := util.PromptItemInList(
			fmt.Sprintf("Enter campaign type (in %v)", typeOptions),
			typeOptions,
		)

		var (
			CampType          uint8
			epochOfTheCam     string
			startTimestampBig *big.Int
			endTimestampBig   *big.Int
			Options           []*big.Int = []*big.Int{}
			noOfOptions       *big.Int
			link              string
		)

		switch cType {
		case "general":
			CampType = 0
		case "fee":
			CampType = 1
		case "brr":
			CampType = 2
		}

		// when creating BRR campaign and KIP2 is used
		if cType == "brr" && KIP2 {
			// startTimestampBig = big.NewInt(int64(timeInfo.NextEpochStartTimestamp))
			// endTimestampBig = big.NewInt(int64(timeInfo.NextEpochStartTimestamp + 10*24*60*60))
			startTimestampBig = big.NewInt(1615446427)
			endTimestampBig = big.NewInt(1616310427)
			Options, err = generateBRROptions(dao, timeInfo.CurrentEpoch-1)
			if err != nil {
				fmt.Printf("Couldn't generate new BRR options: %s\n", err)
			}
			link = "https://github.com/KyberNetwork/KIPs/blob/master/KIPs/kip-2.md"
		} else {
			startTimestampBig = util.PromptNumber(
				"Enter start timestamp for the campaign",
				func(number *big.Int) error {
					if number.Uint64() < timeInfo.CurrentBlockTimestamp {
						return fmt.Errorf("Can't create a campaign with start timestamp in the past")
					}

					if number.Uint64() >= timeInfo.NextEpochEndTimestamp {
						return fmt.Errorf("Can't create a campaign too far in the future (can only create campaigns for current and next epoch)")
					}

					return nil
				},
				config.Network,
			)
			if startTimestampBig.Uint64() < timeInfo.NextEpochStartTimestamp {
				epochOfTheCam = "current"
			} else {
				epochOfTheCam = "next"
			}

			endTimestampBig = util.PromptNumber(
				"Enter end timestamp for the campaign",
				func(number *big.Int) error {
					if number.Uint64()-startTimestampBig.Uint64() < MinCamDurationInSeconds {
						return fmt.Errorf("Can't create a campaign that lasts less than %d seconds", MinCamDurationInSeconds)
					}

					if epochOfTheCam == "current" && number.Uint64() >= timeInfo.NextEpochStartTimestamp {
						return fmt.Errorf("Can't create a campaign that lasts across 2 epochs")
					}

					if epochOfTheCam == "next" && number.Uint64() > timeInfo.NextEpochEndTimestamp {
						return fmt.Errorf("Can't create a campaign that lasts across 2 epochs")
					}
					return nil
				},
				config.Network,
			)

			noOfOptions = util.PromptNumber(
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
			link = util.PromptInput("Enter ref link for the campaign")
		}

		// using msig command to do the tx
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
		config.MethodIndex = 5
		config.ExtraGasLimit = 500000

		config.PrefillStr = fmt.Sprintf(
			// campType|startBlock|endBlock|min|c|t|options|link
			"%d|%d|%d|%d|%d|%d|%s|\"%s\"",
			CampType,
			startTimestampBig,
			endTimestampBig,
			big.NewInt(40000000000000000),
			big.NewInt(0),
			big.NewInt(0),
			optionStr(Options),
			link,
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
	createCamCmd.PersistentFlags().BoolVarP(&KIP2, "kip2", "K", true, "Auto generate BRR campaign params based on KIP2.")
}
