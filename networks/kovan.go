package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var Kovan Network = NewKovan()

type kovan struct {
	*GenericEtherscanNetwork
}

func NewKovan() *kovan {
	return &kovan{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "kovan",
			AlternativeNames:   []string{},
			ChainID:            42,
			NativeTokenSymbol:  "ETH",
			NativeTokenDecimal: 18,
			BlockTime:          2,
			NodeVariableName:   "ETHEREUM_KOVAN_NODE",
			DefaultNodes: map[string]string{
				"kovan-infura": "https://kovan.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.etherscan.io/api",
			MultiCallContractAddress:        common.HexToAddress("0x2cc8688c5f75e365aaeeb4ea8d6a480405a48d2a"),
		}),
	}
}
