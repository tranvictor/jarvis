package networks

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/util/explorers"
)

type GenericOptimismNetworkConfig struct {
	Name                            string            `json:"name"`
	AlternativeNames                []string          `json:"alternative_names"`
	ChainID                         uint64            `json:"chain_id"`
	NativeTokenSymbol               string            `json:"native_token_symbol"`
	NativeTokenDecimal              uint64            `json:"native_token_decimal"`
	BlockTime                       uint64            `json:"block_time"`
	NodeVariableName                string            `json:"node_variable_name"`
	DefaultNodes                    map[string]string `json:"default_nodes"`
	BlockExplorerAPIKeyVariableName string            `json:"block_explorer_api_key_variable_name"`
	BlockExplorerAPIURL             string            `json:"block_explorer_api_url"`
	MultiCallContractAddress        common.Address    `json:"multi_call_contract_address"`
	SyncTxSupported                 bool              `json:"sync_tx_supported"`
}

// GenericOptimismNetwork is a generic implementation of a network that uses Etherscan as their official explorer
type GenericOptimismNetwork struct {
	*explorers.OptimisticRollupExplorer
	config GenericOptimismNetworkConfig
}

func NewGenericOptimismNetwork(config GenericOptimismNetworkConfig) *GenericOptimismNetwork {
	result := &GenericOptimismNetwork{
		OptimisticRollupExplorer: explorers.NewOptimisticRollupExplorer(
			config.BlockExplorerAPIURL,
			strings.Trim(os.Getenv(config.BlockExplorerAPIKeyVariableName), " "),
		),
		config: config,
	}
	return result
}

func (gn *GenericOptimismNetwork) GetName() string {
	return gn.config.Name
}

func (gn *GenericOptimismNetwork) GetChainID() uint64 {
	return gn.config.ChainID
}

func (gn *GenericOptimismNetwork) GetAlternativeNames() []string {
	return gn.config.AlternativeNames
}

func (gn *GenericOptimismNetwork) GetNativeTokenSymbol() string {
	return gn.config.NativeTokenSymbol
}

func (gn *GenericOptimismNetwork) GetNativeTokenDecimal() uint64 {
	return gn.config.NativeTokenDecimal
}

func (gn *GenericOptimismNetwork) GetBlockTime() time.Duration {
	return time.Duration(gn.config.BlockTime) * time.Second
}

func (gn *GenericOptimismNetwork) GetNodeVariableName() string {
	return gn.config.NodeVariableName
}

func (gn *GenericOptimismNetwork) GetDefaultNodes() map[string]string {
	return gn.config.DefaultNodes
}

func (gn *GenericOptimismNetwork) GetBlockExplorerAPIKeyVariableName() string {
	return gn.config.BlockExplorerAPIKeyVariableName
}

func (gn *GenericOptimismNetwork) GetBlockExplorerAPIURL() string {
	return gn.config.BlockExplorerAPIURL
}

func (gn *GenericOptimismNetwork) MultiCallContract() string {
	return gn.config.MultiCallContractAddress.Hex()
}

func (gn *GenericOptimismNetwork) MarshalJSON() ([]byte, error) {
	return json.Marshal(gn.config)
}

func (gn *GenericOptimismNetwork) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &gn.config)
}

func (gn *GenericOptimismNetwork) IsSyncTxSupported() bool {
	return gn.config.SyncTxSupported
}
