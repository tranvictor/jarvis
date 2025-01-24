package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var Rinkeby Network = NewRinkeby()

type rinkeby struct {
	*GenericEtherscanNetwork
}

func NewRinkeby() *rinkeby {
	return &rinkeby{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "rinkeby",
			ChainID:            4,
			NativeTokenSymbol:  "ETH",
			NativeTokenDecimal: 18,
			BlockTime:          2,
			NodeVariableName:   "ETHEREUM_RINKEBY_NODE",
			DefaultNodes: map[string]string{
				"rinkeby-infura": "https://rinkeby.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.etherscan.io/api",
			MultiCallContractAddress:        common.HexToAddress("0x42ad527de7d4e9d9d011ac45b31d8551f8fe9821"),
		}),
	}
}
