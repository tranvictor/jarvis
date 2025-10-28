package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var BaseMainnet Network = NewBaseMainnet()

type baseMainnet struct {
	*GenericEtherscanNetwork
}

func NewBaseMainnet() *baseMainnet {
	return &baseMainnet{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "base",
			AlternativeNames:   []string{},
			ChainID:            8453,
			NativeTokenSymbol:  "ETH",
			NativeTokenDecimal: 18,
			BlockTime:          2,
			NodeVariableName:   "BASE_MAINNET_NODE",
			DefaultNodes: map[string]string{
				"public-base": "https://mainnet.base.org",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.etherscan.io/v2",
			MultiCallContractAddress:        common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11"),
		}),
	}
}

func (b *baseMainnet) IsSyncTxSupported() bool {
	return false
}
