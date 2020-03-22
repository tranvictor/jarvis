package kyberdao

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/config"
)

var (
	StakingContract string
	DaoContract     string
	FeeHandler      string
	KNCContract     string
	CampaignCreator string
	EpochDuration   uint64
	StartDAOBlock   uint64
	MinCamDuration  uint64

	Epoch      uint64
	CampaignID uint64
)

func Preprocess(cmd *cobra.Command, args []string) (err error) {
	switch config.Network {
	case "mainnet":
		StakingContract = ""
		DaoContract = ""
		FeeHandler = ""
		KNCContract = ""
		CampaignCreator = ""
		EpochDuration = 0
		StartDAOBlock = 0
		MinCamDuration = 0
		return fmt.Errorf("'%s' doesn't have kyber staking yet", config.Network)
	case "ropsten":
		StakingContract = "0x8ba3ecae2ffd1dc9e730e54c9c5481d30a3ca2a3"
		DaoContract = "0x752f6BEb3E103696842414Be1f3361011167EDa9"
		FeeHandler = "0x8329e24cb7d85284f32689ef57983c8a7d4b268b"
		KNCContract = "0x4e470dc7321e84ca96fcaedd0c8abcebbaeb68c6"
		CampaignCreator = "0xddf05698718ba8ed1c9aba198d38a825a64d69e2"
		EpochDuration = 200
		StartDAOBlock = 7553600
		MinCamDuration = 10
		return nil
	}
	return fmt.Errorf("'%s' is not support for this app", config.Network)
}
