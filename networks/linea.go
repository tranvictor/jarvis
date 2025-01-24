package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var LineaMainnet Network = NewlineaMainnet()

type lineaMainnet struct {
	*GenericEtherscanNetwork
}

func NewlineaMainnet() *lineaMainnet {
	return &lineaMainnet{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "linea",
			AlternativeNames:   []string{},
			ChainID:            59144,
			NativeTokenSymbol:  "ETH",
			NativeTokenDecimal: 18,
			BlockTime:          2,
			NodeVariableName:   "LINEA_MAINNET_NODE",
			DefaultNodes: map[string]string{
				"infura-linea": "https://linea-mainnet.infura.io/v3/1556a477007b49cda01f9f3df4d97edd",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.etherscan.io/api",
			MultiCallContractAddress:        common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11"),
		}),
	}
}
