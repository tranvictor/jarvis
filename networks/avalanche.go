package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var Avalanche Network = NewAvalanche()

type avalanche struct {
	*GenericEtherscanNetwork
}

func NewAvalanche() *avalanche {
	return &avalanche{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "avalanche",
			AlternativeNames:   []string{"snowtrace"},
			ChainID:            43114,
			NativeTokenSymbol:  "AVAX",
			NativeTokenDecimal: 18,
			BlockTime:          2,
			NodeVariableName:   "AVALANCHE_MAINNET_NODE",
			DefaultNodes: map[string]string{
				"avalanche": "https://api.avax.network/ext/bc/C/rpc",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.snowtrace.io/api",
			MultiCallContractAddress:        common.HexToAddress("0xa00FB557AA68d2e98A830642DBbFA534E8512E5f"),
		}),
	}
}
