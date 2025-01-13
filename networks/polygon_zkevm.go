package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var PolygonZkevmMainnet Network = NewPolygonZkevmMainnet()

type polygonZkevmMainnet struct {
	*EtherscanLikeExplorer
}

func NewPolygonZkevmMainnet() *polygonZkevmMainnet {
	result := &polygonZkevmMainnet{NewEtherscanV2()}
	result.ChainID = result.GetChainID()
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *polygonZkevmMainnet) GetName() string {
	return "zkevm"
}

func (self *polygonZkevmMainnet) GetChainID() uint64 {
	return 1101
}

func (self *polygonZkevmMainnet) GetAlternativeNames() []string {
	return []string{}
}

func (self *polygonZkevmMainnet) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *polygonZkevmMainnet) GetNativeTokenDecimal() uint64 {
	return 18
}

func (self *polygonZkevmMainnet) GetBlockTime() time.Duration {
	return 10 * time.Second
}

func (self *polygonZkevmMainnet) GetNodeVariableName() string {
	return "POLYGON_ZKEVM_MAINNET_NODE"
}

func (self *polygonZkevmMainnet) GetDefaultNodes() map[string]string {
	return map[string]string{
		"public-polygonZkevm": "https://zkevm-rpc.com",
	}
}

func (self *polygonZkevmMainnet) GetBlockExplorerAPIKeyVariableName() string {
	return "POLYGON_ZKEVMSCAN_API_KEY"
}

func (self *polygonZkevmMainnet) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

// func (self *polygonZkevmMainnet) RecommendedGasPrice() (float64, error) {
// 	return 0.01, nil
// }

func (self *polygonZkevmMainnet) MultiCallContract() string {
	return "0xcA11bde05977b3631167028862bE2a173976CA11"
}
