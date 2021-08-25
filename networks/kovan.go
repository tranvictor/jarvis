package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/ethutils/explorers"
)

var Kovan Network = NewKovan()

type kovan struct {
	*EtherscanLikeExplorer
}

func NewKovan() *kovan {
	result := &kovan{NewKovanEtherscan()}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *kovan) GetName() string {
	return "kovan"
}

func (self *kovan) GetChainID() int64 {
	return 42
}

func (self *kovan) GetAlternativeNames() []string {
	return []string{}
}

func (self *kovan) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *kovan) GetNativeTokenDecimal() int64 {
	return 18
}

func (self *kovan) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *kovan) GetNodeVariableName() string {
	return "ETHEREUM_KOVAN_NODE"
}

func (self *kovan) GetDefaultNodes() map[string]string {
	return map[string]string{
		"kovan-infura": "https://kovan.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
}

func (self *kovan) GetBlockExplorerAPIKeyVariableName() string {
	return "ETHERSCAN_API_KEY"
}

func (self *kovan) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

func (self *kovan) MultiCallContract() string {
	return "0x2cc8688c5f75e365aaeeb4ea8d6a480405a48d2a"
}
