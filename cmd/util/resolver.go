package util

import (
	"github.com/ethereum/go-ethereum/accounts/abi"

	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
)

// ABIResolver abstracts address-lookup and ABI-fetching operations that otherwise
// require live network or database access. Command Run functions depend on this
// interface so that tests can substitute a stub and exercise logic offline.
type ABIResolver interface {
	GetAddressFromString(str string) (addr, name string, err error)
	GetMatchingAddress(str string) (addr, name string, err error)
	GetABI(addr string, network jarvisnetworks.Network) (*abi.ABI, error)
	ConfigToABI(addr string, forceERC20 bool, customABI string, network jarvisnetworks.Network) (*abi.ABI, error)
}

// DefaultABIResolver delegates every method to the util package's production
// implementations. It is injected by the preprocessing hooks so that all
// production invocations transparently hit the real databases and Etherscan.
type DefaultABIResolver struct{}

func (DefaultABIResolver) GetAddressFromString(str string) (string, string, error) {
	return util.GetAddressFromString(str)
}

func (DefaultABIResolver) GetMatchingAddress(str string) (string, string, error) {
	return util.GetMatchingAddress(str)
}

func (DefaultABIResolver) GetABI(addr string, network jarvisnetworks.Network) (*abi.ABI, error) {
	return util.GetABI(addr, network)
}

func (DefaultABIResolver) ConfigToABI(addr string, forceERC20 bool, customABI string, network jarvisnetworks.Network) (*abi.ABI, error) {
	return util.ConfigToABI(addr, forceERC20, customABI, network)
}
