package kyberdao

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/util"
)

func PrintENV() {
	fmt.Printf("--ENV-----------------------------------------------------------------------------------------------\n")
	fmt.Printf("Dao contract: %s\n", DaoContract)
	fmt.Printf("Staking contract: %s\n", StakingContract)
	fmt.Printf("FeeHandler contract: %s\n", FeeHandler)
	fmt.Printf("KNC: %s\n", KNCContract)
	fmt.Printf("----------------------------------------------------------------------------------------------------\n")
}

func PrintStakeInformation(cmd *cobra.Command, info *StakeRelatedInfo) {
	isCurrentEpoch := info.CurrentEpoch == info.Epoch

	if info.CurrentEpoch == info.Epoch {
		cmd.Printf("Working with Epoch: %d (current epoch)\n", info.Epoch)
	} else if info.CurrentEpoch > info.Epoch {
		cmd.Printf("Working with Epoch: %d (epoch in the past, current epoch: %d)\n", info.Epoch, info.CurrentEpoch)
	} else {
		cmd.Printf("Working with Epoch: %d (epoch in the future). Abort!\n", info.Epoch)
		return
	}

	cmd.Printf("Staker: %s\n", util.VerboseAddress(info.Staker))
	stakef := ethutils.BigToFloat(info.Stake, 18)
	cmd.Printf("Your stake: %f KNC (%s)\n", stakef, info.Stake)

	balancef := ethutils.BigToFloat(info.Balance, 18)
	if isCurrentEpoch {
		cmd.Printf("Your pending stake (can withdraw without any penalty): %f KNC (%s)\n", ethutils.BigToFloat(info.PendingStake, 18), info.PendingStake)
	}
	cmd.Printf("Available KNC to stake: %f KNC (%s)\n", balancef, info.Balance)

	if strings.ToLower(info.Staker) == strings.ToLower(info.Representative) {
		cmd.Printf("Your representative: None\n")
	} else {
		cmd.Printf("Your representative: %s (Contact him to get your reward if you have some)\n", util.VerboseAddress(info.Representative))
	}
	cmd.Printf("Stake other people delegated to you: %f KNC\n", ethutils.BigToFloat(info.DelegatedStake, 18))
}
