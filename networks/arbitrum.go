package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var ArbitrumMainnet Network = NewArbitrumMainnet()

type arbitrumMainnet struct {
	*EtherscanLikeExplorer
}

func NewArbitrumMainnet() *arbitrumMainnet {
	result := &arbitrumMainnet{NewEtherscanV2()}
	result.ChainID = result.GetChainID()
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *arbitrumMainnet) GetName() string {
	return "arbitrum"
}

func (self *arbitrumMainnet) GetChainID() uint64 {
	return 42161
}

func (self *arbitrumMainnet) GetAlternativeNames() []string {
	return []string{}
}

func (self *arbitrumMainnet) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *arbitrumMainnet) GetNativeTokenDecimal() uint64 {
	return 18
}

func (self *arbitrumMainnet) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *arbitrumMainnet) GetNodeVariableName() string {
	return "ARBITRUM_MAINNET_NODE"
}

func (self *arbitrumMainnet) GetDefaultNodes() map[string]string {
	return map[string]string{
		"infura": "https://arb1.arbitrum.io/rpc",
		// "alchemy-arbitrum": "https://arb-mainnet.g.alchemy.com/v2/PGAWvp9KLZbqjvap-iingGj-Id7HM_Yn",
		// "arbitrum.io":      "https://arb1.arbitrum.io/rpc",
	}
}

func (self *arbitrumMainnet) GetBlockExplorerAPIKeyVariableName() string {
	return "ETHERSCAN_API_KEY"
}

func (self *arbitrumMainnet) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

// func (self *arbitrumMainnet) RecommendedGasPrice() (float64, error) {
// 	return 0.01, nil
// }

func (self *arbitrumMainnet) MultiCallContract() string {
	return "0x80C7DD17B01855a6D2347444a0FCC36136a314de"
}
