package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var BSCMainnet Network = NewBSCMainnet()

type bscMainnet struct {
	*GenericEtherscanNetwork
}

func NewBSCMainnet() *bscMainnet {
	return &bscMainnet{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "bsc",
			AlternativeNames:   []string{},
			ChainID:            56,
			NativeTokenSymbol:  "BNB",
			NativeTokenDecimal: 18,
			BlockTime:          2,
			NodeVariableName:   "BSC_MAINNET_NODE",
			DefaultNodes: map[string]string{
				"binance":  "https://bsc-dataseed.binance.org",
				"defibit":  "https://bsc-dataseed1.defibit.io",
				"ninicoin": "https://bsc-dataseed1.ninicoin.io",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.etherscan.io/v2",
			MultiCallContractAddress:        common.HexToAddress("0x41263cba59eb80dc200f3e2544eda4ed6a90e76c"),
		}),
	}
}

func (b *bscMainnet) IsSyncTxSupported() bool {
	return false
}
