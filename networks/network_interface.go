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

	IsSyncTxSupported() bool

	// this interface can return "" in case
	// there is no multicall contract on the network
	MultiCallContract() string

	// since network is a persistent object, we need to implement MarshalJSON and UnmarshalJSON
	MarshalJSON() ([]byte, error)
	UnmarshalJSON([]byte) error
}
