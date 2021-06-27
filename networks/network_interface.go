package networks

import (
	"time"
)

type Network interface {
	GetName() string
	GetChainID() int64
	GetAlternativeNames() []string
	GetNativeTokenSymbol() string
	GetNativeTokenDecimal() int64
	GetBlockTime() time.Duration // in second

	GetNodeVariableName() string
	GetDefaultNodes() map[string]string

	GetBlockExplorerAPIKeyVariableName() string
	GetBlockExplorerAPIURL() string
	RecommendedGasPrice() (float64, error)
	GetABIString(address string) (string, error)
}
