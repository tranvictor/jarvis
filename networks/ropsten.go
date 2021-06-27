package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/ethutils/explorers"
)

var Ropsten Network = NewRopsten()

type ropsten struct {
	*EtherscanLikeExplorer
}

func NewRopsten() *ropsten {
	result := &ropsten{NewRopstenEtherscan()}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *ropsten) GetName() string {
	return "ropsten"
}

func (self *ropsten) GetChainID() int64 {
	return 3
}

func (self *ropsten) GetAlternativeNames() []string {
	return []string{}
}

func (self *ropsten) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *ropsten) GetNativeTokenDecimal() int64 {
	return 18
}

func (self *ropsten) GetBlockTime() time.Duration {
	return 14 * time.Second
}

func (self *ropsten) GetNodeVariableName() string {
	return "ETHEREUM_ROPSTEN_NODE"
}

func (self *ropsten) GetDefaultNodes() map[string]string {
	return map[string]string{
		"ropsten-infura": "https://ropsten.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
}

func (self *ropsten) GetBlockExplorerAPIKeyVariableName() string {
	return "ETHERSCAN_API_KEY"
}

func (self *ropsten) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}
