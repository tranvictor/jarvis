package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var EthereumMainnet Network = NewEthereumMainnet()

type ethereumMainnet struct {
	*EtherscanLikeExplorer
}

func NewEthereumMainnet() *ethereumMainnet {
	result := &ethereumMainnet{NewMainnetEtherscan()}
	result.ChainID = result.GetChainID()
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *ethereumMainnet) GetName() string {
	return "mainnet"
}

func (self *ethereumMainnet) GetChainID() uint64 {
	return 1
}

func (self *ethereumMainnet) GetAlternativeNames() []string {
	return []string{"ethereum"}
}

func (self *ethereumMainnet) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *ethereumMainnet) GetNativeTokenDecimal() uint64 {
	return 18
}

func (self *ethereumMainnet) GetBlockTime() time.Duration {
	return 14 * time.Second
}

func (self *ethereumMainnet) GetNodeVariableName() string {
	return "ETHEREUM_MAINNET_NODE"
}

func (self *ethereumMainnet) GetDefaultNodes() map[string]string {
	return map[string]string{
		"mainnet-kyber": "https://ethereum.kyberengineering.io",
	}
}

func (self *ethereumMainnet) GetBlockExplorerAPIKeyVariableName() string {
	return "ETHERSCAN_API_KEY"
}

func (self *ethereumMainnet) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

func (self *ethereumMainnet) MultiCallContract() string {
	return "0xeefba1e63905ef1d7acba5a8513c70307c1ce441"
}
