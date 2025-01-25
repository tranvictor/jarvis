package networks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

// Insert more Network implementation here to support
// more chains
var supportedNetworks = []Network{
	EthereumMainnet,
	Ropsten,
	Kovan,
	Rinkeby,
	// TomoMainnet,
	BSCMainnet,
	BSCTestnet,
	Matic,
	Avalanche,
	Fantom,
	OptimismMainnet,
	ArbitrumMainnet,
	BttcMainnet,
	EthereumPOW,
	ScrollMainnet,
	BaseMainnet,
	PolygonZkevmMainnet,
	// Mumbai,
	LineaMainnet,
	BitfiTestnet,
}

var globalSupportedNetworks = newSupportedNetworks()
var ErrNetworkNotFound = fmt.Errorf("network not found")

type networks struct {
	networks     map[string]Network
	networksByID map[uint64]Network
}

func (n *networks) getSupportedNetworkNames() []string {
	res := []string{}
	for _, n := range n.networks {
		res = append(res, n.GetName())
		res = append(res, n.GetAlternativeNames()...)
	}
	return res
}

func (n *networks) getNetworkByID(id uint64) (Network, error) {
	res, found := n.networksByID[id]
	if !found {
		return nil, fmt.Errorf("network id %d is not supported", id)
	}
	return res, nil
}

func (n *networks) getNetwork(name string) (Network, error) {
	res, found := n.networks[name]
	if !found {
		return nil, fmt.Errorf("network name '%s': %w", name, ErrNetworkNotFound)
	}
	return res, nil
}

func newSupportedNetworks() *networks {
	result := networks{
		map[string]Network{},
		map[uint64]Network{},
	}
	for _, n := range supportedNetworks {
		if _, found := result.networks[n.GetName()]; found {
			panic(
				fmt.Errorf(
					"network with name or alternative name of '%s' already exists",
					n.GetName(),
				),
			)
		}
		result.networks[n.GetName()] = n
		result.networksByID[n.GetChainID()] = n
		for _, an := range n.GetAlternativeNames() {
			if _, found := result.networks[an]; found {
				panic(
					fmt.Errorf("network with name or alternative name of '%s' already exists", an),
				)
			}
			result.networks[an] = n
		}
	}

	// load custom networks from ~/.jarvis/networks/
	customNetworks, err := loadCustomNetworks()
	if err != nil {
		fmt.Printf("WARNING: Failed to load custom networks: %s. Ignore and continue with built-in networks.\n", err)
		return &result
	}

	for _, n := range customNetworks {
		_, nameFound := result.networks[n.GetName()]
		if nameFound {
			fmt.Printf("Network with name '%s' already exists. Using custom network.\n", n.GetName())
		}
		_, idFound := result.networksByID[n.GetChainID()]
		if idFound {
			fmt.Printf("Network with id '%d' already exists. Using custom network.\n", n.GetChainID())
		}
		result.networks[n.GetName()] = n
		result.networksByID[n.GetChainID()] = n
	}
	return &result
}

func loadCustomNetworks() ([]Network, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	customNetworksDir := filepath.Join(usr.HomeDir, ".jarvis", "networks")
	files, err := filepath.Glob(filepath.Join(customNetworksDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob between json files in ./jarvis/networks: %w", err)
	}

	networks := []Network{}

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", file, err)
		}

		network, err := NewNetworkFromJSON(content)
		if err != nil {
			fmt.Printf("failed to parse network from file %s: %s. Ignore and continue with other custom networks.\n", file, err)
			continue
		}

		networks = append(networks, network)
	}

	return networks, nil
}

func NewNetworkFromJSON(content []byte) (Network, error) {
	networkConfig := GenericEtherscanNetworkConfig{}
	err := json.Unmarshal(content, &networkConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal network config: %w", err)
	}

	return NewGenericEtherscanNetwork(networkConfig), nil
}

func GetSupportedNetworks() []Network {
	res := []Network{}
	for _, n := range globalSupportedNetworks.networks {
		res = append(res, n)
	}
	return res
}

func GetNetwork(name string) (Network, error) {
	return globalSupportedNetworks.getNetwork(name)
}

func GetNetworkByID(id uint64) (Network, error) {
	return globalSupportedNetworks.getNetworkByID(id)
}

func GetSupportedNetworkNames() []string {
	return globalSupportedNetworks.getSupportedNetworkNames()
}

func AddNetwork(network Network) error {
	globalSupportedNetworks.networks[network.GetName()] = network
	globalSupportedNetworks.networksByID[network.GetChainID()] = network

	for _, an := range network.GetAlternativeNames() {
		if _, found := globalSupportedNetworks.networks[an]; found {
			panic(
				fmt.Errorf("network with name or alternative name of '%s' already exists", an),
			)
		}
		globalSupportedNetworks.networks[an] = network
	}

	// store the new network to ~/.jarvis/networks/
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	customNetworksDir := filepath.Join(usr.HomeDir, ".jarvis", "networks")
	os.MkdirAll(customNetworksDir, 0755)

	content, err := network.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal network: %w", err)
	}

	err = os.WriteFile(filepath.Join(customNetworksDir, fmt.Sprintf("%s.json", network.GetName())), content, 0644)
	if err != nil {
		return fmt.Errorf("failed to write the new network to file: %w", err)
	}

	return nil
}
