package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/ethutils/explorers"
)

var BSCMainnet Network = NewBSCMainnet()

type bscMainnet struct {
	*EtherscanLikeExplorer
}

func NewBSCMainnet() *bscMainnet {
	result := &bscMainnet{NewBscscan()}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *bscMainnet) GetName() string {
	return "bsc"
}

func (self *bscMainnet) GetChainID() int64 {
	return 56
}

func (self *bscMainnet) GetAlternativeNames() []string {
	return []string{}
}

func (self *bscMainnet) GetNativeTokenSymbol() string {
	return "BNB"
}

func (self *bscMainnet) GetNativeTokenDecimal() int64 {
	return 18
}

func (self *bscMainnet) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *bscMainnet) GetNodeVariableName() string {
	return "BSC_MAINNET_NODE"
}

func (self *bscMainnet) GetDefaultNodes() map[string]string {
	return map[string]string{
		"binance":  "https://bsc-dataseed.binance.org",
		"defibit":  "https://bsc-dataseed1.defibit.io",
		"ninicoin": "https://bsc-dataseed1.ninicoin.io",
	}
}

func (self *bscMainnet) GetBlockExplorerAPIKeyVariableName() string {
	return "BSCSCAN_API_KEY"
}

func (self *bscMainnet) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

func (self *bscMainnet) RecommendedGasPrice() (float64, error) {
	return 10, nil
}

func (self *bscMainnet) MultiCallContract() string {
	return "0x41263cba59eb80dc200f3e2544eda4ed6a90e76c"
}
