package networks

import (
	"fmt"
	"sync"
)

var (
	cachedNetwork Network
	mu            sync.Mutex
)

func CurrentNetwork() Network {
	if cachedNetwork != nil {
		return cachedNetwork
	}

	SetNetwork(NetworkString)

	return cachedNetwork
}

func SetNetwork(networkStr string) {
	mu.Lock()
	defer mu.Unlock()

	var err error
	var inited bool

	if cachedNetwork != nil {
		inited = true
	}

	cachedNetwork, err = GetNetwork(networkStr)
	if err != nil {
		cachedNetwork = EthereumMainnet
	} else {
		if inited {
			fmt.Printf("Switched to network: %s\n", cachedNetwork.GetName())
		} else {
			fmt.Printf("Network: %s\n", cachedNetwork.GetName())
		}
	}
}

var NetworkString string
