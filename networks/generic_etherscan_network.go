package networks

import (
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/util/explorers"
)

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
}

// GenericEtherscanNetwork is a generic implementation of a network that uses Etherscan as their official explorer
type GenericEtherscanNetwork struct {
	*explorers.EtherscanLikeExplorer
	config GenericEtherscanNetworkConfig
}

func NewGenericEtherscanNetwork(config GenericEtherscanNetworkConfig) *GenericEtherscanNetwork {
	result := &GenericEtherscanNetwork{
		EtherscanLikeExplorer: explorers.NewEtherscanV2(),
		config:                config,
	}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (gn *GenericEtherscanNetwork) GetName() string {
	return gn.config.Name
}

func (gn *GenericEtherscanNetwork) GetChainID() uint64 {
	return gn.config.ChainID
}

func (gn *GenericEtherscanNetwork) GetAlternativeNames() []string {
	return gn.config.AlternativeNames
}

func (gn *GenericEtherscanNetwork) GetNativeTokenSymbol() string {
	return gn.config.NativeTokenSymbol
}

func (gn *GenericEtherscanNetwork) GetNativeTokenDecimal() uint64 {
	return gn.config.NativeTokenDecimal
}

func (gn *GenericEtherscanNetwork) GetBlockTime() time.Duration {
	return time.Duration(gn.config.BlockTime) * time.Second
}

func (gn *GenericEtherscanNetwork) GetNodeVariableName() string {
	return gn.config.NodeVariableName
}

func (gn *GenericEtherscanNetwork) GetDefaultNodes() map[string]string {
	return gn.config.DefaultNodes
}

func (gn *GenericEtherscanNetwork) GetBlockExplorerAPIKeyVariableName() string {
	return gn.config.BlockExplorerAPIKeyVariableName
}

func (gn *GenericEtherscanNetwork) GetBlockExplorerAPIURL() string {
	return gn.config.BlockExplorerAPIURL
}

func (gn *GenericEtherscanNetwork) MultiCallContract() string {
	return gn.config.MultiCallContractAddress.Hex()
}
