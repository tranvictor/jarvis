package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var Avalanche Network = NewAvalanche()

type avalanche struct {
	*EtherscanLikeExplorer
}

func NewAvalanche() *avalanche {
	result := &avalanche{NewSnowtrace()}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *avalanche) GetName() string {
	return "avalanche"
}

func (self *avalanche) GetChainID() uint64 {
	return 43114
}

func (self *avalanche) GetAlternativeNames() []string {
	return []string{"snowtrace"}
}

func (self *avalanche) GetNativeTokenSymbol() string {
	return "AVAX"
}

func (self *avalanche) GetNativeTokenDecimal() uint64 {
	return 18
}

func (self *avalanche) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *avalanche) GetNodeVariableName() string {
	return "AVALANCHE_MAINNET_NODE"
}

func (self *avalanche) GetDefaultNodes() map[string]string {
	return map[string]string{
		"avalanche": "https://api.avax.network/ext/bc/C/rpc",
	}
}

func (self *avalanche) GetBlockExplorerAPIKeyVariableName() string {
	return "SNOWTRACE_API_KEY"
}

func (self *avalanche) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

func (self *avalanche) MultiCallContract() string {
	return "0xa00FB557AA68d2e98A830642DBbFA534E8512E5f"
}
