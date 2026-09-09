package networks

import (
	"os"
	"strings"

	"github.com/tranvictor/jarvis/util/explorers"
)

// GenericOptimismNetwork is a network whose explorer is Blockscout-style
// (Optimistic rollup /v2 smart-contract API) rather than Etherscan.
type GenericOptimismNetwork struct {
	*explorers.OptimisticRollupExplorer
	networkMeta
}

func NewGenericOptimismNetwork(config GenericEtherscanNetworkConfig) *GenericOptimismNetwork {
	return &GenericOptimismNetwork{
		OptimisticRollupExplorer: explorers.NewOptimisticRollupExplorer(
			config.BlockExplorerAPIURL,
			strings.Trim(os.Getenv(config.BlockExplorerAPIKeyVariableName), " "),
		),
		networkMeta: networkMeta{Config: config},
	}
}
