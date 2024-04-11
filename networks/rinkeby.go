package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var Rinkeby Network = NewRinkeby()

type rinkeby struct {
	*EtherscanLikeExplorer
}

func NewRinkeby() *rinkeby {
	result := &rinkeby{NewRinkebyEtherscan()}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *rinkeby) GetName() string {
	return "rinkeby"
}

func (self *rinkeby) GetChainID() uint64 {
	return 4
}

func (self *rinkeby) GetAlternativeNames() []string {
	return []string{}
}

func (self *rinkeby) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *rinkeby) GetNativeTokenDecimal() uint64 {
	return 18
}

func (self *rinkeby) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *rinkeby) GetNodeVariableName() string {
	return "ETHEREUM_RINKEBY_NODE"
}

func (self *rinkeby) GetDefaultNodes() map[string]string {
	return map[string]string{
		"rinkeby-infura": "https://rinkeby.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
}

func (self *rinkeby) GetBlockExplorerAPIKeyVariableName() string {
	return "ETHERSCAN_API_KEY"
}

func (self *rinkeby) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

func (self *rinkeby) MultiCallContract() string {
	return "0x42ad527de7d4e9d9d011ac45b31d8551f8fe9821"
}
