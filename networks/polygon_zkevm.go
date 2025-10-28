package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var PolygonZkevmMainnet Network = NewPolygonZkevmMainnet()

type polygonZkevmMainnet struct {
	*GenericEtherscanNetwork
}

func NewPolygonZkevmMainnet() *polygonZkevmMainnet {
	return &polygonZkevmMainnet{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "polygon-zkevm",
			ChainID:            1101,
			NativeTokenSymbol:  "ETH",
			NativeTokenDecimal: 18,
			BlockTime:          10,
			NodeVariableName:   "POLYGON_ZKEVM_MAINNET_NODE",
			DefaultNodes: map[string]string{
				"public-polygonZkevm": "https://zkevm-rpc.com",
			},
			BlockExplorerAPIKeyVariableName: "POLYGON_ZKEVMSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.etherscan.io/v2",
			MultiCallContractAddress:        common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11"),
		}),
	}
}

func (p *polygonZkevmMainnet) IsSyncTxSupported() bool {
	return false
}
