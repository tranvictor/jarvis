package networks

import (
	"time"
)

type Network interface {
	GetName() string
	GetChainID() uint64
	GetAlternativeNames() []string
	GetNativeTokenSymbol() string
	GetNativeTokenDecimal() uint64
	GetBlockTime() time.Duration // in second

	GetNodeVariableName() string
	GetDefaultNodes() map[string]string

	GetBlockExplorerAPIKeyVariableName() string
	GetBlockExplorerAPIURL() string
	RecommendedGasPrice() (float64, error)
	GetABIString(address string) (string, error)

	// this interface can return "" in case
	// there is no multicall contract on the network
	MultiCallContract() string
}
