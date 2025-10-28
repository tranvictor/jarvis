package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var Matic Network = NewMatic()

type matic struct {
	*GenericEtherscanNetwork
}

func NewMatic() *matic {
	return &matic{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "matic",
			AlternativeNames:   []string{"polygon"},
			ChainID:            137,
			NativeTokenSymbol:  "MATIC",
			NativeTokenDecimal: 18,
			BlockTime:          2,
			NodeVariableName:   "MATIC_MAINNET_NODE",
			DefaultNodes: map[string]string{
				"kyber": "https://polygon.kyberengineering.io",
			},
			BlockExplorerAPIKeyVariableName: "ETHERSCAN_API_KEY",
			BlockExplorerAPIURL:             "https://api.etherscan.io/v2",
			MultiCallContractAddress:        common.HexToAddress("0x11ce4B23bD875D7F5C6a31084f55fDe1e9A87507"),
		}),
	}
}

func (m *matic) IsSyncTxSupported() bool {
	return false
}
