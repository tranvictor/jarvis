package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var Mumbai Network = NewMumbai()

type mumbai struct {
	*GenericEtherscanNetwork
}

func NewMumbai() *mumbai {
	return &mumbai{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "mumbai",
			AlternativeNames:   []string{"polygon-testnet", "matic-testnet"},
			ChainID:            80001,
			NativeTokenSymbol:  "MATIC",
			NativeTokenDecimal: 18,
			BlockTime:          2,
			NodeVariableName:   "MATIC_TESTNET_NODE",
			DefaultNodes: map[string]string{
				"infura-mumbai": "https://polygon-mumbai.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.etherscan.io/v2",
			MultiCallContractAddress:        common.HexToAddress("0x08411ADd0b5AA8ee47563b146743C13b3556c9Cc"),
		}),
	}
}

func (m *mumbai) IsSyncTxSupported() bool {
	return false
}
