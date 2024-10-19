package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var BitfiTestnet = NewBitfiTestnet()

type bitfiTestnet struct {
	*OptimisticRollupExplorer
}

func NewBitfiTestnet() *bitfiTestnet {
	result := &bitfiTestnet{NewOptimisticRollupExplorer(
		"https://bitfi-ledger-testnet-explorer.alt.technology/api/v2",
		"",
	)}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.OptimisticRollupExplorer.APIKey = apiKey
	}
	return result
}

func (self *bitfiTestnet) GetName() string {
	return "bitfi-testnet"
}

func (self *bitfiTestnet) GetChainID() uint64 {
	return 891891
}

func (self *bitfiTestnet) GetAlternativeNames() []string {
	return []string{}
}

func (self *bitfiTestnet) GetNativeTokenSymbol() string {
	return "ETH"
}

func (self *bitfiTestnet) GetNativeTokenDecimal() uint64 {
	return 18
}

func (self *bitfiTestnet) GetBlockTime() time.Duration {
	return 1 * time.Second
}

func (self *bitfiTestnet) GetNodeVariableName() string {
	return "BITFI_TESTNET_NODE"
}

func (self *bitfiTestnet) GetDefaultNodes() map[string]string {
	return map[string]string{
		"public-bitfi-testnet": "https://bitfi-ledger-testnet.alt.technology",
	}
}

func (self *bitfiTestnet) GetBlockExplorerAPIKeyVariableName() string {
	return "BITFI_TESTNET_SCAN_API_KEY"
}

func (self *bitfiTestnet) GetBlockExplorerAPIURL() string {
	return self.OptimisticRollupExplorer.Domain
}

func (self *bitfiTestnet) MultiCallContract() string {
	return "0xcA11bde05977b3631167028862bE2a173976CA11"
}
