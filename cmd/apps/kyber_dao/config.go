package kyberdao

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

var (
	StakingContract string
	DaoContract     string
	FeeHandler      string
	KNCContract     string

	Epoch      uint64
	CampaignID uint64
)

func Preprocess(cmd *cobra.Command, args []string) (err error) {
	config.From, _, err = util.GetAddressFromString(config.From)
	if err != nil {
		return fmt.Errorf("Couldn't interpret addresss. Please double check your -f flag. %w\n", err)
	}

	switch config.Network {
	case "mainnet":
		StakingContract = ""
		DaoContract = ""
		FeeHandler = ""
		KNCContract = ""
		return fmt.Errorf("'%s' doesn't have kyber staking yet", config.Network)
	case "ropsten":
		StakingContract = "0x4a78660e83b01a3f50196678018fa2efe1932401"
		DaoContract = "0x3f740889a810b244aff37b88bbbf2c685b848eb1"
		FeeHandler = "0x99770684ca992b816256d6e92f3b8e3b490514a6"
		KNCContract = "0x4e470dc7321e84ca96fcaedd0c8abcebbaeb68c6"
		return nil
	}
	return fmt.Errorf("'%s' is not support for this app", config.Network)
}
