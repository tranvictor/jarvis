package common

import (
	"github.com/ethereum/go-ethereum/accounts/abi"

	jarvisnetworks "github.com/tranvictor/jarvis/networks"
)

// type AddressDatabase interface {
// 	GetName(addr string) string
// }

type ABIDatabase func(address string, network jarvisnetworks.Network) (*abi.ABI, error)
