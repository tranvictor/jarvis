package networks

import (
	"os"
	"strings"

	"github.com/tranvictor/jarvis/util/explorers"
)

// GenericOptimismNetwork is a network whose explorer is Blockscout-style
// (Optimistic rollup /v2 smart-contract API) rather than Etherscan.
type GenericOptimismNetwork struct {
	*explorers.EtherscanLikeExplorer
	networkMeta
}

func NewGenericOptimismNetwork(config GenericEtherscanNetworkConfig) *GenericOptimismNetwork {
	kind := explorers.KindBlockscout
	if k, err := explorers.ParseKind(config.ExplorerKind); err == nil && k != "" {
		kind = k
	}
	return &GenericOptimismNetwork{
		EtherscanLikeExplorer: explorers.New(
			kind,
			config.BlockExplorerAPIURL,
			strings.Trim(os.Getenv(config.BlockExplorerAPIKeyVariableName), " "),
			config.ChainID,
		),
		networkMeta: networkMeta{Config: config},
	}
}
