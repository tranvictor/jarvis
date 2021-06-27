package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/ethutils/explorers"
)

var Mumbai Network = NewMumbai()

type mumbai struct {
	*EtherscanLikeExplorer
}

func NewMumbai() *mumbai {
	result := &mumbai{NewMumbaiPolygonscan()}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *mumbai) GetName() string {
	return "mumbai"
}

func (self *mumbai) GetChainID() int64 {
	return 80001
}

func (self *mumbai) GetAlternativeNames() []string {
	return []string{"polygon-testnet", "matic-testnet"}
}

func (self *mumbai) GetNativeTokenSymbol() string {
	return "MATIC"
}

func (self *mumbai) GetNativeTokenDecimal() int64 {
	return 18
}

func (self *mumbai) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *mumbai) GetNodeVariableName() string {
	return "MATIC_TESTNET_NODE"
}

func (self *mumbai) GetDefaultNodes() map[string]string {
	return map[string]string{
		"infura": "https://polygon-mumbai.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
}

func (self *mumbai) GetBlockExplorerAPIKeyVariableName() string {
	return "POLYGONSCAN_API_KEY"
}

func (self *mumbai) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}
