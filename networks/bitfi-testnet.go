package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

var BitfiTestnet = NewBitfiTestnet()

type bitfiTestnet struct {
	*GenericEtherscanNetwork
}

func NewBitfiTestnet() *bitfiTestnet {
	return &bitfiTestnet{
		GenericEtherscanNetwork: NewGenericEtherscanNetwork(GenericEtherscanNetworkConfig{
			Name:               "bitfi-testnet",
			AlternativeNames:   []string{},
			ChainID:            891891,
			NativeTokenSymbol:  "ETH",
			NativeTokenDecimal: 18,
			BlockTime:          1,
			NodeVariableName:   "BITFI_TESTNET_NODE",
			DefaultNodes: map[string]string{
				"public-bitfi-testnet":  "https://bitfi-ledger-testnet.alt.technology",
				"caliber-bitfi-testnet": " https://rpc2-testnet.bitfi.xyz",
			},
			BlockExplorerAPIKeyVariableName: "BITFI_TESTNET_SCAN_API_KEY",
			BlockExplorerAPIURL:             "https://bitfi-ledger-testnet-explorer.alt.technology/api/v2",
			MultiCallContractAddress:        common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11"),
		}),
	}
}
