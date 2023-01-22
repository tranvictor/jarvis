package networks

import (
	"fmt"
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
	// Mumbai,
}

var globalSupportedNetworks = newSupportedNetworks()

type networks struct {
	networks map[string]Network
}

func (n *networks) getSupportedNetworkNames() []string {
	res := []string{}
	for _, n := range supportedNetworks {
		res = append(res, n.GetName())
		res = append(res, n.GetAlternativeNames()...)
	}
	return res
}

func (n *networks) getNetwork(name string) (Network, error) {
	res, found := n.networks[name]
	if !found {
		return nil, fmt.Errorf("network name %s is not supported", name)
	}
	return res, nil
}

func newSupportedNetworks() *networks {
	result := networks{
		map[string]Network{},
	}
	for _, n := range supportedNetworks {
		if _, found := result.networks[n.GetName()]; found {
			panic(fmt.Errorf("network with name or alternative name of %s already exists", n.GetName()))
		}
		result.networks[n.GetName()] = n
		for _, an := range n.GetAlternativeNames() {
			if _, found := result.networks[an]; found {
				panic(fmt.Errorf("network with name or alternative name of %s already exists", an))
			}
			result.networks[an] = n
		}
	}
	return &result
}

func GetSupportedNetworks() []Network {
	return supportedNetworks
}

func GetNetwork(name string) (Network, error) {
	return globalSupportedNetworks.getNetwork(name)
}

func GetSupportedNetworkNames() []string {
	return globalSupportedNetworks.getSupportedNetworkNames()
}
