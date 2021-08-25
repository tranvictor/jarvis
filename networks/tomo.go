package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/ethutils/explorers"
)

var TomoMainnet Network = NewTomoMainnet()

type tomoMainnet struct {
	*EtherscanLikeExplorer
}

func NewTomoMainnet() *tomoMainnet {
	result := &tomoMainnet{NewTomoBlockExplorer()}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *tomoMainnet) GetName() string {
	return "tomo"
}

func (self *tomoMainnet) GetChainID() int64 {
	return 88
}

func (self *tomoMainnet) GetAlternativeNames() []string {
	return []string{}
}

func (self *tomoMainnet) GetNativeTokenSymbol() string {
	return "TOMO"
}

func (self *tomoMainnet) GetNativeTokenDecimal() int64 {
	return 18
}

func (self *tomoMainnet) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *tomoMainnet) GetNodeVariableName() string {
	return "TOMO_MAINNET_NODE"
}

func (self *tomoMainnet) GetDefaultNodes() map[string]string {
	return map[string]string{
		"mainnet-tomo": "https://rpc.tomochain.com",
	}
}

func (self *tomoMainnet) GetBlockExplorerAPIKeyVariableName() string {
	return "TOMOSCAN_API_KEY"
}

func (self *tomoMainnet) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

func (self *tomoMainnet) MultiCallContract() string {
	return ""
}
