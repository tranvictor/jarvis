package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var ArbitrumMainnet Network = NewArbitrumMainnet()

type arbitrumMainnet struct {
	*GenericEtherscanNetwork
}

func NewArbitrumMainnet() *arbitrumMainnet {
	return &arbitrumMainnet{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "arbitrum",
			ChainID:            42161,
			NativeTokenSymbol:  "ETH",
			NativeTokenDecimal: 18,
			BlockTime:          2,
			NodeVariableName:   "ARBITRUM_MAINNET_NODE",
			DefaultNodes: map[string]string{
				"infura": "https://arb1.arbitrum.io/rpc",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.etherscan.io/v2",
			MultiCallContractAddress:        common.HexToAddress("0x80C7DD17B01855a6D2347444a0FCC36136a314de"),
		}),
	}
}

func (a *arbitrumMainnet) IsSyncTxSupported() bool {
	return true
}
