package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var BSCTestnet Network = NewBSCTestnet()

type bscTestnet struct {
	*GenericEtherscanNetwork
}

func NewBSCTestnet() *bscTestnet {
	return &bscTestnet{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "bsc-testnet",
			ChainID:            97,
			NativeTokenSymbol:  "BNB",
			NativeTokenDecimal: 18,
			BlockTime:          2,
			NodeVariableName:   "BSC_TESTNET_NODE",
			DefaultNodes: map[string]string{
				"binance1": "https://data-seed-prebsc-1-s1.binance.org:8545",
				"binance2": "https://data-seed-prebsc-2-s1.binance.org:8545",
				"binance3": "https://data-seed-prebsc-1-s2.binance.org:8545",
				"binance4": "https://data-seed-prebsc-2-s2.binance.org:8545",
				"binance5": "https://data-seed-prebsc-1-s3.binance.org:8545",
				"binance6": "https://data-seed-prebsc-2-s3.binance.org:8545",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.bscscan.com/api",
			MultiCallContractAddress:        common.HexToAddress("0xae11C5B5f29A6a25e955F0CB8ddCc416f522AF5C"),
		}),
	}
}
