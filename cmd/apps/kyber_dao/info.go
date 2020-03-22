package kyberdao

import (
	"fmt"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

var (
	Stake        *big.Int
	PendingStake *big.Int
	Delegation   string
)

var infoCmd = &cobra.Command{
	Use:              "staker-info",
	Short:            "Show stake, your reward and current voting campaigns",
	TraverseChildren: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		if len(args) < 1 {
			fmt.Errorf("Please specified staker address via param. Abort.\n")
			return
		}

		config.From, _, err = util.GetAddressFromString(args[0])
		if err != nil {
			return fmt.Errorf("Couldn't interpret addresss. Please double check your -f flag. %w\n", err)
		}

		return Preprocess(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		PrintENV()
		reader, err := util.EthReader(config.Network)
		if err != nil {
			fmt.Printf("Couldn't init eth reader: %s\n", err)
			return
		}

		dao := NewKyberDAO(reader, StakingContract, DaoContract, FeeHandler)
		stakeInfo, err := dao.AllStakeRelatedInfo(config.From, Epoch)
		if err != nil {
			cmd.Printf("Couldn't get stake information: %s\n", err)
			return
		}
		Epoch = stakeInfo.Epoch

		PrintStakeInformation(cmd, stakeInfo)

		fmt.Printf("\nYour REWARD including your delegators' (during last 5 epochs):\n")
		for i := uint64(0); i < 5 && Epoch >= i; i++ {
			e := Epoch - i
			reward, totalReward, share, isClaimed, err := dao.GetRewardInfo(config.From, e)
			if err != nil {
				fmt.Printf("Couldn't get reward info: %s\n", err)
				return
			}
			if isClaimed {
				fmt.Printf("%d - %f ETH - %f%% of total reward pool (%f ETH) | CLAIMED\n", e, ethutils.BigToFloat(reward, 18), share, ethutils.BigToFloat(totalReward, 18))
			} else {
				fmt.Printf("%d - %f ETH - %f%% of total reward pool (%f ETH)\n", e, ethutils.BigToFloat(reward, 18), share, ethutils.BigToFloat(totalReward, 18))
			}
		}

		camIDs, err := dao.GetCampaignIDs(Epoch)
		// camIDs, err := dao.GetCampaignIDs(1)
		fmt.Printf("\nThere are %d voting campaigns for epoch %d:\n", len(camIDs), Epoch)

		currentBlock, err := reader.CurrentBlock()
		if err != nil {
			fmt.Printf("Couldn't get current block: %s\n", err)
			return
		}

		for _, id := range camIDs {
			campaignRelatedInfo, err := dao.AllCampaignRelatedInfo(config.From, id)
			if err != nil {
				cmd.Printf("Couldn't get data of campaign %d: %s\n", id, err)
				return
			}
			PrintCampaignInformation(cmd, campaignRelatedInfo, currentBlock)
		}
	},
}

func init() {
	infoCmd.PersistentFlags().Uint64VarP(&Epoch, "epoch", "e", 0, "Epoch to read staking and dao data.")
}
