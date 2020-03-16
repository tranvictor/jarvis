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
	Use:               "staker-info",
	Short:             "Show stake, your reward and current voting campaigns",
	TraverseChildren:  true,
	PersistentPreRunE: Preprocess,
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
			cam, err := dao.GetCampaignDetail(id)
			if err != nil {
				fmt.Printf("Couldn't get campaign (%d) details: %s\n", id, err)
				return
			}
			votedID, err := dao.GetVotedOptionID(config.From, id)
			if err != nil {
				fmt.Printf("Couldn't get voted options for campaign (%d): %s\n", id, err)
				return
			}
			// CampaignType campType, uint startBlock, uint endBlock,
			// uint totalKNCSupply, uint formulaParams, bytes memory link, uint[] memory options
			fmt.Printf("----------------------------------------------------------------------------------------------------\n")
			fmt.Printf("Campaign ID: %d\n", id)
			fmt.Printf("Type: %s\n", cam.Type())
			fmt.Printf("Duration: block %d -> %d, %d blocks\n",
				cam.StartBlock.Uint64(),
				cam.EndBlock.Uint64(),
				cam.EndBlock.Uint64()-cam.StartBlock.Uint64())
			timeLeft := util.CalculateTimeDurationFromBlock(config.Network, currentBlock, cam.EndBlock.Uint64())
			if timeLeft == 0 {
				fmt.Printf("Time left: ENDED\n")
			} else {
				fmt.Printf("Time left: %s\n", timeLeft.String())
			}
			if len(cam.LinkStr()) == 0 {
				fmt.Printf("For more information: No link is provided.\n")
			} else {
				fmt.Printf("For more information: %s\n", cam.LinkStr())
			}
			fmt.Printf("\n%d options to vote:\n", len(cam.Options))
			for i, op := range cam.Options {
				if votedID.Int64() == int64(i+1) {
					fmt.Printf("    %d. %s (you voted)\n", i+1, cam.VerboseOption(op))
				} else {
					fmt.Printf("    %d. %s\n", i+1, cam.VerboseOption(op))
				}
			}
			fmt.Printf("\n")
		}
	},
}

func init() {
	infoCmd.PersistentFlags().StringVarP(&config.From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
	infoCmd.PersistentFlags().Uint64VarP(&Epoch, "epoch", "e", 0, "Epoch to read staking and dao data.")
}
