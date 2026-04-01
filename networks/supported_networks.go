package networks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
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
		name := n.GetName()
		if strings.TrimSpace(name) != "" {
			res = append(res, name)
		}
		for _, an := range n.GetAlternativeNames() {
			if strings.TrimSpace(an) != "" {
				res = append(res, an)
			}
		}
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

	for _, entry := range customNetworks {
		n := entry.network
		_, nameFound := result.networks[n.GetName()]
		_, idFound := result.networksByID[n.GetChainID()]
		if nameFound || idFound {
			var reasons []string
			if nameFound {
				reasons = append(reasons, fmt.Sprintf("name %q", n.GetName()))
			}
			if idFound {
				reasons = append(reasons, fmt.Sprintf("chain id %d", n.GetChainID()))
			}
			fmt.Printf("Note: Custom network file %q overlaps a bundled network (%s).\n", entry.path, strings.Join(reasons, " and "))
			fmt.Printf("      Jarvis uses the RPC and explorer settings from that file. Remove or rename the file to use bundled defaults; you can ignore this if the override is intentional.\n")
		}
		result.networks[n.GetName()] = n
		result.networksByID[n.GetChainID()] = n
	}
	return &result
}

type customNetworkFile struct {
	network Network
	path    string
}

func loadCustomNetworks() ([]customNetworkFile, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	customNetworksDir := filepath.Join(usr.HomeDir, ".jarvis", "networks")
	files, err := filepath.Glob(filepath.Join(customNetworksDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob between json files in ./jarvis/networks: %w", err)
	}

	var out []customNetworkFile

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

		out = append(out, customNetworkFile{network: network, path: file})
	}

	return out, nil
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
	// The internal map stores one entry per name AND per alternative name, so
	// the same network object appears multiple times. Deduplicate by chain ID.
	seen := map[uint64]bool{}
	res := []Network{}
	for _, n := range globalSupportedNetworks.networks {
		if !seen[n.GetChainID()] {
			seen[n.GetChainID()] = true
			res = append(res, n)
		}
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
