package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/ethutils/explorers"
)

var BSCTestnet Network = NewBSCTestnet()

type bscTestnet struct {
	*EtherscanLikeExplorer
}

func NewBSCTestnet() *bscTestnet {
	result := &bscTestnet{NewTestnetBscscan()}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *bscTestnet) GetName() string {
	return "bsc-test"
}

func (self *bscTestnet) GetChainID() int64 {
	return 97
}

func (self *bscTestnet) GetAlternativeNames() []string {
	return []string{"bsc-testnet"}
}

func (self *bscTestnet) GetNativeTokenSymbol() string {
	return "BNB"
}

func (self *bscTestnet) GetNativeTokenDecimal() int64 {
	return 18
}

func (self *bscTestnet) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *bscTestnet) GetNodeVariableName() string {
	return "BSC_TESTNET_NODE"
}

func (self *bscTestnet) GetDefaultNodes() map[string]string {
	return map[string]string{
		"binance1": "https://data-seed-prebsc-1-s1.binance.org:8545",
		"binance2": "https://data-seed-prebsc-2-s1.binance.org:8545",
		"binance3": "https://data-seed-prebsc-1-s2.binance.org:8545",
		"binance4": "https://data-seed-prebsc-2-s2.binance.org:8545",
		"binance5": "https://data-seed-prebsc-1-s3.binance.org:8545",
		"binance6": "https://data-seed-prebsc-2-s3.binance.org:8545",
	}
}

func (self *bscTestnet) GetBlockExplorerAPIKeyVariableName() string {
	return "BSCSCAN_API_KEY"
}

func (self *bscTestnet) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}
