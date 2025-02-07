package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var EthereumMainnet Network = NewEthereumMainnet()

type ethereumMainnet struct {
	*GenericEtherscanNetwork
}

func NewEthereumMainnet() *ethereumMainnet {
	return &ethereumMainnet{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "mainnet",
			AlternativeNames:   []string{"ethereum"},
			ChainID:            1,
			NativeTokenSymbol:  "ETH",
			NativeTokenDecimal: 18,
			BlockTime:          14,
			NodeVariableName:   "ETHEREUM_MAINNET_NODE",
			DefaultNodes: map[string]string{
				"mainnet-kyber": "https://ethereum.kyberengineering.io",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.etherscan.io/v2",
			MultiCallContractAddress:        common.HexToAddress("0xeefba1e63905ef1d7acba5a8513c70307c1ce441"),
		}),
	}
}
