package config

import (
	"fmt"
	"math/big"
	"sync"

	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/networks"
)

var (
	cachedNetwork networks.Network
	mu            sync.Mutex
)

func Network() networks.Network {
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

	cachedNetwork, err = networks.GetNetwork(networkStr)
	if err != nil {
		cachedNetwork = networks.EthereumMainnet
	} else {
		if inited {
			fmt.Printf("Switched to network: %s\n", cachedNetwork.GetName())
		} else {
			fmt.Printf("Network: %s\n", cachedNetwork.GetName())
		}
	}
}

var NetworkString string
var Debug bool
var DegenMode bool

var (
	GasPrice             float64
	ExtraGasPrice        float64
	GasLimit             uint64
	ExtraGasLimit        uint64
	Nonce                uint64
	From                 string
	FromAcc              accounts.AccDesc
	To                   string
	Value                *big.Int
	RawValue             string
	MethodIndex          uint64
	PrefillMode          bool
	PrefillStr           string
	PrefillParams        []string
	NoFuncCall           bool
	Tx                   string
	TxInfo               *common.TxInfo
	AllZeroParamsMethods bool
	AtBlock              int64

	MsigValue float64
	MsigTo    string

	DontBroadcast     bool
	DontWaitToBeMined bool
	ForceERC20ABI     bool
	RetryBroadcast    bool
	YesToAllPrompt    bool

	CustomABI      string
	JSONOutputFile string
)
