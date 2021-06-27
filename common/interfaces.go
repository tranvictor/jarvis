package common

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	. "github.com/tranvictor/jarvis/networks"
)

// type AddressDatabase interface {
// 	GetName(addr string) string
// }

type ABIDatabase func(address string, network Network) (*abi.ABI, error)
