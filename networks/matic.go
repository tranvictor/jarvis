package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var Matic Network = NewMatic()

type matic struct {
	*EtherscanLikeExplorer
}

func NewMatic() *matic {
	result := &matic{NewPolygonscan()}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	result.ChainID = result.GetChainID()
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *matic) GetName() string {
	return "matic"
}

func (self *matic) GetChainID() uint64 {
	return 137
}

func (self *matic) GetAlternativeNames() []string {
	return []string{"polygon"}
}

func (self *matic) GetNativeTokenSymbol() string {
	return "MATIC"
}

func (self *matic) GetNativeTokenDecimal() uint64 {
	return 18
}

func (self *matic) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *matic) GetNodeVariableName() string {
	return "MATIC_MAINNET_NODE"
}

func (self *matic) GetDefaultNodes() map[string]string {
	return map[string]string{
		"kyber": "https://polygon.kyberengineering.io",
	}
}

func (self *matic) GetBlockExplorerAPIKeyVariableName() string {
	return "ETHERSCAN_API_KEY"
}

func (self *matic) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

// func (self *matic) RecommendedGasPrice() (float64, error) {
// 	return 10, nil
// }

func (self *matic) MultiCallContract() string {
	return "0x11ce4B23bD875D7F5C6a31084f55fDe1e9A87507"
}
