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
	// SafeTxServiceURL is optional: chains that Safe doesn't list in its
	// own registry (private/custom chains especially) can point jarvis at
	// a self-hosted Safe Transaction Service here. omitempty keeps it out
	// of the JSON of the bundled networks, which all leave it unset.
	SafeTxServiceURL string `json:"safe_tx_service_url,omitempty"`
}

// networkMeta holds the JSON-serializable chain config and implements the
// Network methods that don't depend on the explorer backend. Both
// GenericEtherscanNetwork and GenericOptimismNetwork embed it.
type networkMeta struct {
	Config GenericEtherscanNetworkConfig
}

func (m *networkMeta) GetName() string {
	return m.Config.Name
}

func (m *networkMeta) GetChainID() uint64 {
	return m.Config.ChainID
}

func (m *networkMeta) GetAlternativeNames() []string {
	return m.Config.AlternativeNames
}

func (m *networkMeta) GetNativeTokenSymbol() string {
	return m.Config.NativeTokenSymbol
}

func (m *networkMeta) GetNativeTokenDecimal() uint64 {
	return m.Config.NativeTokenDecimal
}

func (m *networkMeta) GetBlockTime() time.Duration {
	return time.Duration(m.Config.BlockTime) * time.Second
}

func (m *networkMeta) GetNodeVariableName() string {
	return m.Config.NodeVariableName
}

func (m *networkMeta) GetDefaultNodes() map[string]string {
	return m.Config.DefaultNodes
}

func (m *networkMeta) GetBlockExplorerAPIKeyVariableName() string {
	return m.Config.BlockExplorerAPIKeyVariableName
}

func (m *networkMeta) GetBlockExplorerAPIURL() string {
	return m.Config.BlockExplorerAPIURL
}

// GetSafeTxServiceURL normalises the configured URL the same way
// txservice does for the env overrides: trimmed, with no trailing slash,
// so callers can concatenate paths onto it unconditionally.
func (m *networkMeta) GetSafeTxServiceURL() string {
	return strings.TrimRight(strings.TrimSpace(m.Config.SafeTxServiceURL), "/")
}

func (m *networkMeta) MultiCallContract() string {
	return m.Config.MultiCallContractAddress.Hex()
}

func (m *networkMeta) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Config)
}

func (m *networkMeta) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &m.Config)
}

// GenericEtherscanNetwork is a generic implementation of a network that uses Etherscan as their official explorer
type GenericEtherscanNetwork struct {
	*explorers.EtherscanLikeExplorer
	networkMeta
}

func NewGenericEtherscanNetwork(config GenericEtherscanNetworkConfig) *GenericEtherscanNetwork {
	apiKey := strings.Trim(os.Getenv(config.BlockExplorerAPIKeyVariableName), " ")
	if apiKey == "" {
		apiKey = defaultAPIKey
	}
	return &GenericEtherscanNetwork{
		EtherscanLikeExplorer: explorers.NewEtherscanLikeExplorer(config.BlockExplorerAPIURL, apiKey, config.ChainID),
		networkMeta:           networkMeta{Config: config},
	}
}
