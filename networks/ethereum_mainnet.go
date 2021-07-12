package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/ethutils/explorers"
)

var EthereumMainnet Network = NewEthereumMainnet()

type ethereumMainnet struct {
	*EtherscanLikeExplorer
}

func NewEthereumMainnet() *ethereumMainnet {
	result := &ethereumMainnet{NewMainnetEtherscan()}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *ethereumMainnet) GetName() string {
	return "mainnet"
}

func (self *ethereumMainnet) GetChainID() int64 {
	return 1
}

func (self *ethereumMainnet) GetAlternativeNames() []string {
	return []string{}
}

func (self *ethereumMainnet) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *ethereumMainnet) GetNativeTokenDecimal() int64 {
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
		"mainnet-alchemy": "https://eth-mainnet.alchemyapi.io/v2/YP5f6eM2wC9c2nwJfB0DC1LObdSY7Qfv",
		"mainnet-infura":  "https://mainnet.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
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
