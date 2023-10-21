package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var BaseMainnet Network = NewBaseMainnet()

type baseMainnet struct {
	*EtherscanLikeExplorer
}

func NewBaseMainnet() *baseMainnet {
	result := &baseMainnet{NewEtherscanLikeExplorer(
		"https://api.basescan.org",
		"KGU9B686UEDCRMMMMEDA3Q4M4EM9GMTVKA",
	)}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *baseMainnet) GetName() string {
	return "base"
}

func (self *baseMainnet) GetChainID() int64 {
	return 8453
}

func (self *baseMainnet) GetAlternativeNames() []string {
	return []string{}
}

func (self *baseMainnet) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *baseMainnet) GetNativeTokenDecimal() int64 {
	return 18
}

func (self *baseMainnet) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *baseMainnet) GetNodeVariableName() string {
	return "BASE_MAINNET_NODE"
}

func (self *baseMainnet) GetDefaultNodes() map[string]string {
	return map[string]string{
		"public-base": "https://mainnet.base.org",
	}
}

func (self *baseMainnet) GetBlockExplorerAPIKeyVariableName() string {
	return "BASESCAN_API_KEY"
}

func (self *baseMainnet) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

// func (self *baseMainnet) RecommendedGasPrice() (float64, error) {
// 	return 0.01, nil
// }

func (self *baseMainnet) MultiCallContract() string {
	return "0xcA11bde05977b3631167028862bE2a173976CA11"
}
