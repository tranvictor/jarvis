package networks

import (
	"time"

	"github.com/tranvictor/jarvis/util/explorers"
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
	GetContractInfo(address string) (explorers.ContractInfo, error)

	IsSyncTxSupported() bool

	// GetSafeTxServiceURL returns the Safe Transaction Service base URL
	// this network is configured with, or "" when it has none. It is
	// consulted after the SAFE_TX_SERVICE_URL[_<chainID>] env overrides
	// but before Safe's own chain registry, so a custom chain can carry
	// its service alongside the rest of its config instead of relying on
	// an env var being exported in every shell.
	GetSafeTxServiceURL() string

	// this interface can return "" in case
	// there is no multicall contract on the network
	MultiCallContract() string

	// since network is a persistent object, we need to implement MarshalJSON and UnmarshalJSON
	MarshalJSON() ([]byte, error)
	UnmarshalJSON([]byte) error
}
