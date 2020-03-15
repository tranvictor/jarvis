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
	StakingContract string
	DaoContract     string
	FeeHandler      string

	Epoch        uint64
	Stake        *big.Int
	PendingStake *big.Int
	Delegation   string
)

func Preprocess(cmd *cobra.Command, args []string) (err error) {
	switch config.Network {
	case "mainnet":
		StakingContract = ""
		DaoContract = ""
		return fmt.Errorf("'%s' doesn't have kyber staking yet", config.Network)
	case "ropsten":
		StakingContract = "0x4a78660e83b01a3f50196678018fa2efe1932401"
		DaoContract = "0x3f740889a810b244aff37b88bbbf2c685b848eb1"
		FeeHandler = "0x99770684ca992b816256d6e92f3b8e3b490514a6"
		return nil
	}
	return fmt.Errorf("'%s' is not support for this app", config.Network)
}

var infoCmd = &cobra.Command{
	Use:               "info",
	Short:             "Show stake, your reward and current voting campaigns",
	TraverseChildren:  true,
	PersistentPreRunE: Preprocess,
	Run: func(cmd *cobra.Command, args []string) {
		reader, err := util.EthReader(config.Network)
		if err != nil {
			fmt.Printf("Couldn't init eth reader: %s\n", err)
			return
		}
		config.From, _, err = util.GetAddressFromString(config.From)
		if err != nil {
			fmt.Printf("Couldn't interpret from address: %s\n", err)
			return
		}
		dao := NewKyberDAO(reader, StakingContract, DaoContract, FeeHandler)
		var isCurrentEpoch bool
		if Epoch == 0 {
			Epoch, err = dao.CurrentEpoch()
			if err != nil {
				fmt.Printf("Couldn't get current epoch: %s\n", err)
				return
			}
			isCurrentEpoch = true
		}
		if isCurrentEpoch {
			fmt.Printf("Working with Epoch: %d (latest epoch)\n", Epoch)
		} else {
			fmt.Printf("Working with Epoch: %d (epoch in the past)\n", Epoch)
		}
		fmt.Printf("Staker: %s\n", util.VerboseAddress(config.From))
		stake, err := dao.GetStake(config.From, Epoch)
		if err != nil {
			fmt.Printf("Couldn't get stake of %s at epoch %d: %s\n", config.From, Epoch, err)
			return
		}
		fmt.Printf("Your stake: %f KNC (%s)\n", ethutils.BigToFloat(stake, 18), stake)
		if isCurrentEpoch {
			futureStake, err := dao.GetStake(config.From, Epoch+1)
			if err != nil {
				fmt.Printf("Couldn't get stake of %s at epoch %d: %s\n", config.From, Epoch, err)
				return
			}
			pendingStake := big.NewInt(0).Sub(futureStake, stake)
			fmt.Printf("Your pending stake (can withdraw without any penalty): %f KNC (%s)\n", ethutils.BigToFloat(pendingStake, 18), pendingStake)
		}
		poolMaster, err := dao.GetPoolMaster(config.From, Epoch)
		if err != nil {
			fmt.Printf("Couldn't get pool master info of %s at epoch %d: %s\n", config.From, Epoch, err)
			return
		}
		fmt.Printf("Your pool master: %s\n", util.VerboseAddress(poolMaster.Hex()))
		delegatedStake, err := dao.GetDelegatedStake(config.From, Epoch)
		if err != nil {
			fmt.Printf("Couldn't get delegated stake of %s at epoch %d: %s\n", config.From, Epoch, err)
			return
		}
		fmt.Printf("Stake other people delegated to you: %f KNC\n", ethutils.BigToFloat(delegatedStake, 18))

		fmt.Printf("\nYour REWARD (during last 5 epochs):\n")
		for i := uint64(0); i < 5 && Epoch >= i; i++ {
			e := Epoch - i
			reward, totalReward, share, err := dao.GetRewardInfo(config.From, e)
			if err != nil {
				fmt.Printf("Couldn't get reward info: %s\n", err)
				return
			}
			fmt.Printf("%d - %f ETH - %f%% of total reward pool (%f ETH)\n", e, ethutils.BigToFloat(reward, 18), share, ethutils.BigToFloat(totalReward, 18))
		}

		// camIDs, err := dao.GetCampaignIDs(Epoch)
		camIDs, err := dao.GetCampaignIDs(1)
		fmt.Printf("\nThere are %d voting campaigns for current epoch:\n", len(camIDs))
		for _, id := range camIDs {
			cam, err := dao.GetCampaignDetail(id)
			if err != nil {
				fmt.Printf("Couldn't get campaign (%d) details: %s\n", id, err)
				return
			}
			votedOption, err := dao.GetVotedOption(config.From, id)
			if err != nil {
				fmt.Printf("Couldn't get voted options for campaign (%d): %s\n", id, err)
				return
			}
			// CampaignType campType, uint startBlock, uint endBlock,
			// uint totalKNCSupply, uint formulaParams, bytes memory link, uint[] memory options
			fmt.Printf("-- Campaign %d:\n", id)
			fmt.Printf("   Type %s:\n", cam.Type())
			fmt.Printf("   Duration: block %d -> %d, %d blocks\n")
			fmt.Printf("   Time left: %s\n", "not implemented yet")
			fmt.Printf("   For more information: %s\n", cam.LinkStr())
			fmt.Printf("   %d Options:", len(cam.Options))
			for i, op := range cam.Options {
				if votedOption.Int64() == int64(i+1) {
					fmt.Printf("    %d. %s (you voted)\n", i, cam.VerboseOption(op))
				} else {
					fmt.Printf("    %d. %s\n", i, cam.VerboseOption(op))
				}
			}
			fmt.Printf("\n")
		}
	},
}

func init() {
	infoCmd.PersistentFlags().StringVarP(&config.From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
	infoCmd.PersistentFlags().Uint64VarP(&Epoch, "epoch", "e", 0, "Epoch to read staking and dao data.")

	infoCmd.MarkFlagRequired("from")
}
