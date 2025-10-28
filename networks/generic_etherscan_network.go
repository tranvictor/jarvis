package networks

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/util/explorers"
)

const defaultAPIKey = "UBB257TI824FC7HUSPT66KZUMGBPRN3IWV"

type GenericEtherscanNetworkConfig struct {
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
	SyncedTxSupported               bool              `json:"synced_tx_supported"`
}

// GenericEtherscanNetwork is a generic implementation of a network that uses Etherscan as their official explorer
type GenericEtherscanNetwork struct {
	*explorers.EtherscanLikeExplorer
	Config GenericEtherscanNetworkConfig
}

func NewGenericEtherscanNetwork(config GenericEtherscanNetworkConfig) *GenericEtherscanNetwork {
	apiKey := strings.Trim(os.Getenv(config.BlockExplorerAPIKeyVariableName), " ")
	if apiKey == "" {
		apiKey = defaultAPIKey
	}
	result := &GenericEtherscanNetwork{
		EtherscanLikeExplorer: explorers.NewEtherscanLikeExplorer(config.BlockExplorerAPIURL, apiKey, config.ChainID),
		Config:                config,
	}
	return result
}

func (gn *GenericEtherscanNetwork) GetName() string {
	return gn.Config.Name
}

func (gn *GenericEtherscanNetwork) GetChainID() uint64 {
	return gn.Config.ChainID
}

func (gn *GenericEtherscanNetwork) GetAlternativeNames() []string {
	return gn.Config.AlternativeNames
}

func (gn *GenericEtherscanNetwork) GetNativeTokenSymbol() string {
	return gn.Config.NativeTokenSymbol
}

func (gn *GenericEtherscanNetwork) GetNativeTokenDecimal() uint64 {
	return gn.Config.NativeTokenDecimal
}

func (gn *GenericEtherscanNetwork) GetBlockTime() time.Duration {
	return time.Duration(gn.Config.BlockTime) * time.Second
}

func (gn *GenericEtherscanNetwork) GetNodeVariableName() string {
	return gn.Config.NodeVariableName
}

func (gn *GenericEtherscanNetwork) GetDefaultNodes() map[string]string {
	return gn.Config.DefaultNodes
}

func (gn *GenericEtherscanNetwork) GetBlockExplorerAPIKeyVariableName() string {
	return gn.Config.BlockExplorerAPIKeyVariableName
}

func (gn *GenericEtherscanNetwork) GetBlockExplorerAPIURL() string {
	return gn.Config.BlockExplorerAPIURL
}

func (gn *GenericEtherscanNetwork) MultiCallContract() string {
	return gn.Config.MultiCallContractAddress.Hex()
}

func (gn *GenericEtherscanNetwork) MarshalJSON() ([]byte, error) {
	return json.Marshal(gn.Config)
}

func (gn *GenericEtherscanNetwork) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &gn.Config)
}

func (gn *GenericEtherscanNetwork) IsSyncTxSupported() bool {
	return gn.Config.SyncedTxSupported
}
