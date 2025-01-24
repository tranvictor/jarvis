package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var EthereumPOW Network = NewEthereumPOW()

type ethereumPOW struct {
	*GenericEtherscanNetwork
}

func NewEthereumPOW() *ethereumPOW {
	return &ethereumPOW{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "ethpow",
			AlternativeNames:   []string{},
			ChainID:            10001,
			NativeTokenSymbol:  "ETH",
			NativeTokenDecimal: 18,
			BlockTime:          14,
			NodeVariableName:   "ETHEREUM_POW_NODE",
			DefaultNodes: map[string]string{
				"ethpow-team": "https://mainnet.ethereumpow.org",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.etherscan.io/api",
			MultiCallContractAddress:        common.HexToAddress("0xeefba1e63905ef1d7acba5a8513c70307c1ce441"),
		}),
	}
}
