package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/ethutils/explorers"
)

var EthereumPOW Network = NewEthereumPOW()

type ethereumPOW struct {
	*EtherscanLikeExplorer
}

func NewEthereumPOW() *ethereumPOW {
	result := &ethereumPOW{NewMainnetEtherscan()}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *ethereumPOW) GetName() string {
	return "ethpow"
}

func (self *ethereumPOW) GetChainID() int64 {
	return 10001
}

func (self *ethereumPOW) GetAlternativeNames() []string {
	return []string{}
}

func (self *ethereumPOW) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *ethereumPOW) GetNativeTokenDecimal() int64 {
	return 18
}

func (self *ethereumPOW) GetBlockTime() time.Duration {
	return 14 * time.Second
}

func (self *ethereumPOW) GetNodeVariableName() string {
	return "ETHEREUM_POW_NODE"
}

func (self *ethereumPOW) GetDefaultNodes() map[string]string {
	return map[string]string{
		"ethpow-team": "https://mainnet.ethereumpow.org",
	}
}

func (self *ethereumPOW) GetBlockExplorerAPIKeyVariableName() string {
	return "ETHERPOWSCAN_API_KEY"
}

func (self *ethereumPOW) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

func (self *ethereumPOW) MultiCallContract() string {
	return "0xeefba1e63905ef1d7acba5a8513c70307c1ce441"
}
