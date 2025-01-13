package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var LineaMainnet Network = NewlineaMainnet()

type lineaMainnet struct {
	*EtherscanLikeExplorer
}

func NewlineaMainnet() *lineaMainnet {
	result := &lineaMainnet{NewEtherscanV2()}
	result.ChainID = result.GetChainID()
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *lineaMainnet) GetName() string {
	return "linea"
}

func (self *lineaMainnet) GetChainID() uint64 {
	return 59144
}

func (self *lineaMainnet) GetAlternativeNames() []string {
	return []string{}
}

func (self *lineaMainnet) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *lineaMainnet) GetNativeTokenDecimal() uint64 {
	return 18
}

func (self *lineaMainnet) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *lineaMainnet) GetNodeVariableName() string {
	return "LINEA_MAINNET_NODE"
}

func (self *lineaMainnet) GetDefaultNodes() map[string]string {
	return map[string]string{
		"infura-linea": "https://linea-mainnet.infura.io/v3/1556a477007b49cda01f9f3df4d97edd",
	}
}

func (self *lineaMainnet) GetBlockExplorerAPIKeyVariableName() string {
	return "ETHERSCAN_API_KEY"
}

func (self *lineaMainnet) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

// func (self *lineaMainnet) RecommendedGasPrice() (float64, error) {
// 	return 0.01, nil
// }

func (self *lineaMainnet) MultiCallContract() string {
	return "0xcA11bde05977b3631167028862bE2a173976CA11"
}
