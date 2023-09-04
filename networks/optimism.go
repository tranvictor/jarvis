package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var OptimismMainnet Network = NewOptimismMainnet()

type optimismMainnet struct {
	*EtherscanLikeExplorer
}

func NewOptimismMainnet() *optimismMainnet {
	result := &optimismMainnet{NewEtherscanLikeExplorer("https://api-optimistic.etherscan.io", "RU33HVN77Q51YNQIANP7GXZRAZ423CETB8")}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *optimismMainnet) GetName() string {
	return "optimism"
}

func (self *optimismMainnet) GetChainID() uint64 {
	return 10
}

func (self *optimismMainnet) GetAlternativeNames() []string {
	return []string{}
}

func (self *optimismMainnet) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *optimismMainnet) GetNativeTokenDecimal() uint64 {
	return 18
}

func (self *optimismMainnet) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *optimismMainnet) GetNodeVariableName() string {
	return "OPTIMISM_MAINNET_NODE"
}

func (self *optimismMainnet) GetDefaultNodes() map[string]string {
	return map[string]string{
		"mainnet-optimism": "https://mainnet.optimism.io",
	}
}

func (self *optimismMainnet) GetBlockExplorerAPIKeyVariableName() string {
	return "OPTIMISTIC_ETHERSCAN_API_KEY"
}

func (self *optimismMainnet) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

// func (self *optimismMainnet) RecommendedGasPrice() (float64, error) {
// 	return 0.00001, nil
// }

func (self *optimismMainnet) MultiCallContract() string {
	return "0xD9bfE9979e9CA4b2fe84bA5d4Cf963bBcB376974"
}
