package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var ScrollMainnet Network = NewScrollMainnet()

type scrollMainnet struct {
	*GenericEtherscanNetwork
}

func NewScrollMainnet() *scrollMainnet {
	return &scrollMainnet{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "scroll",
			AlternativeNames:   []string{},
			ChainID:            534352,
			NativeTokenSymbol:  "ETH",
			NativeTokenDecimal: 18,
			BlockTime:          3,
			NodeVariableName:   "SCROLL_MAINNET_NODE",
			DefaultNodes: map[string]string{
				"public-scroll": "https://rpc.scroll.io",
			},
			BlockExplorerAPIKeyVariableName: "SCROLLSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.scrollscan.com/api",
			MultiCallContractAddress:        common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11"),
		}),
	}
}
