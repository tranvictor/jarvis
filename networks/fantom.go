package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var Fantom Network = NewFantom()

type fantom struct {
	*GenericEtherscanNetwork
}

func NewFantom() *fantom {
	return &fantom{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "fantom",
			AlternativeNames:   []string{"ftm"},
			ChainID:            250,
			NativeTokenSymbol:  "FTM",
			NativeTokenDecimal: 18,
			BlockTime:          1,
			NodeVariableName:   "FANTOM_MAINNET_NODE",
			DefaultNodes: map[string]string{
				"fantom": "https://rpc.ftm.tools/",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.ftmscan.com/api",
			MultiCallContractAddress:        common.HexToAddress("0xcf591ce5574258aC4550D96c545e4F3fd49A74ec"),
		}),
	}
}
