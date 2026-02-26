package config

import (
	"sync"

	"github.com/tranvictor/jarvis/networks"
)

var (
	Debug     bool = true
	DegenMode bool
)

var (
	cachedNetwork networks.Network
	mu            sync.Mutex
)

func Network() networks.Network {
	if cachedNetwork != nil {
		return cachedNetwork
	}

	err := SetNetwork(NetworkString)
	if err != nil {
		panic(err)
	}

	return cachedNetwork
}

func SetNetwork(networkStr string) error {
	mu.Lock()
	defer mu.Unlock()

	var err error
	cachedNetwork, err = networks.GetNetwork(networkStr)
	return err
}

var NetworkString string

var (
	GasPrice      float64
	ExtraGasPrice float64
	TipGas        float64
	ExtraTipGas   float64
	GasLimit      uint64
	ExtraGasLimit uint64
	Nonce         uint64
	From          string
	To            string
	RawValue      string
	MethodIndex   uint64
	PrefillStr    string
	NoFuncCall    bool
	Tx            string

	AllZeroParamsMethods bool
	AtBlock              int64

	MsigValue float64
	MsigTo    string

	DontBroadcast     bool
	DontWaitToBeMined bool
	ForceERC20ABI     bool
	RetryBroadcast    bool
	YesToAllPrompt    bool
	ForceLegacy       bool

	CustomABI      string
	JSONOutputFile string

	Simulate bool
)
