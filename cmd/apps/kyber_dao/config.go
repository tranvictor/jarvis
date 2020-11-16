package kyberdao

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/config"
)

var (
	StakingContract         string
	DaoContract             string
	FeeHandler              string
	KNCContract             string
	CampaignCreator         string
	EpochDurationInSeconds  uint64
	StartDAOTimestamp       uint64
	MinCamDurationInSeconds uint64

	Epoch      uint64
	CampaignID uint64
)

func Preprocess(cmd *cobra.Command, args []string) (err error) {
	switch config.Network {
	case "mainnet":
		StakingContract = "0xecf0bdb7b3f349abfd68c3563678124c5e8aaea3"
		DaoContract = "0x49bdd8854481005bba4acebabf6e06cd5f6312e9"
		FeeHandler = "0xd3d2b5643e506c6d9b7099e9116d7aaa941114fe"
		KNCContract = "0xdd974d5c2e2928dea5f71b9825b8b646686bd200"
		CampaignCreator = "0xe6a7338cba0a1070adfb22c07115299605454713"
		EpochDurationInSeconds = 1209600
		StartDAOTimestamp = 1594710427
		MinCamDurationInSeconds = 345600
		return nil
	case "ropsten":
		StakingContract = "0x8ba3ecae2ffd1dc9e730e54c9c5481d30a3ca2a3"
		DaoContract = "0x752f6BEb3E103696842414Be1f3361011167EDa9"
		FeeHandler = "0x8329e24cb7d85284f32689ef57983c8a7d4b268b"
		KNCContract = "0x4e470dc7321e84ca96fcaedd0c8abcebbaeb68c6"
		CampaignCreator = "0xddf05698718ba8ed1c9aba198d38a825a64d69e2"
		EpochDurationInSeconds = 0
		StartDAOTimestamp = 0
		MinCamDurationInSeconds = 0
		return fmt.Errorf("'%s' doesn't have kyber staking yet", config.Network)
	}
	return fmt.Errorf("'%s' is not support for this app", config.Network)
}
