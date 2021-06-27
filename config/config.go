package config

import (
	"math/big"

	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/networks"
)

func Network() networks.Network {
	res, err := networks.GetNetwork(NetworkString)
	if err != nil {
		return networks.EthereumMainnet
	}
	return res
}

var NetworkString string

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
	TxInfo               *ethutils.TxInfo
	AllZeroParamsMethods bool
	AtBlock              int64

	MsigValue float64
	MsigTo    string

	DontBroadcast     bool
	DontWaitToBeMined bool
	ForceERC20ABI     bool
	CustomABI         string
	JSONOutputFile    string
)
