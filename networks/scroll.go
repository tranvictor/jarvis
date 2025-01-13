package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var ScrollMainnet Network = NewScrollMainnet()

type scrollMainnet struct {
	*EtherscanLikeExplorer
}

func NewScrollMainnet() *scrollMainnet {
	result := &scrollMainnet{NewEtherscanV2()}
	result.ChainID = result.GetChainID()
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *scrollMainnet) GetName() string {
	return "scroll"
}

func (self *scrollMainnet) GetChainID() uint64 {
	return 534352
}

func (self *scrollMainnet) GetAlternativeNames() []string {
	return []string{}
}

func (self *scrollMainnet) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *scrollMainnet) GetNativeTokenDecimal() uint64 {
	return 18
}

func (self *scrollMainnet) GetBlockTime() time.Duration {
	return 3 * time.Second
}

func (self *scrollMainnet) GetNodeVariableName() string {
	return "SCROLL_MAINNET_NODE"
}

func (self *scrollMainnet) GetDefaultNodes() map[string]string {
	return map[string]string{
		"public-scroll": "https://rpc.scroll.io",
	}
}

func (self *scrollMainnet) GetBlockExplorerAPIKeyVariableName() string {
	return "SCROLLSCAN_API_KEY"
}

func (self *scrollMainnet) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

// func (self *scrollMainnet) RecommendedGasPrice() (float64, error) {
// 	return 0.01, nil
// }

func (self *scrollMainnet) MultiCallContract() string {
	return "0xcA11bde05977b3631167028862bE2a173976CA11"
}
