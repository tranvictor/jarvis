package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var BttcMainnet Network = NewBttcMainnet()

type bttcMainnet struct {
	*EtherscanLikeExplorer
}

func NewBttcMainnet() *bttcMainnet {
	result := &bttcMainnet{NewEtherscanLikeExplorer(
		"https://api.bttcscan.com",
		"W56TCSJ96BRMTU7ZZ4HNEETMIDM8CHCJYR",
	)}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *bttcMainnet) GetName() string {
	return "bttc"
}

func (self *bttcMainnet) GetChainID() int64 {
	return 199
}

func (self *bttcMainnet) GetAlternativeNames() []string {
	return []string{}
}

func (self *bttcMainnet) GetNativeTokenSymbol() string {
	return "BTT"
}

func (self *bttcMainnet) GetNativeTokenDecimal() int64 {
	return 18
}

func (self *bttcMainnet) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *bttcMainnet) GetNodeVariableName() string {
	return "BTTC_MAINNET_NODE"
}

func (self *bttcMainnet) GetDefaultNodes() map[string]string {
	return map[string]string{
		"bt.io": "https://rpc.bt.io",
	}
}

func (self *bttcMainnet) GetBlockExplorerAPIKeyVariableName() string {
	return "BTTCSCAN_API_KEY"
}

func (self *bttcMainnet) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

// func (self *bttcMainnet) RecommendedGasPrice() (float64, error) {
// 	return 0.01, nil
// }

func (self *bttcMainnet) MultiCallContract() string {
	return "0xBF69a56D35B8d6f5A8e0e96B245a72F735751e54"
}
