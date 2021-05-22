package common

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
)

// type AddressDatabase interface {
// 	GetName(addr string) string
// }

type ABIDatabase func(address string, network string) (*abi.ABI, error)
