package config

import (
	"math/big"

	"github.com/tranvictor/jarvis/accounts/types"
)

var (
	Debug     bool
	DegenMode bool
)

var (
	GasPrice      float64
	TipGas        float64
	ExtraGasPrice float64
	GasLimit      uint64
	ExtraGasLimit uint64
	Nonce         uint64
	From          string
	FromAcc       types.AccDesc
	To            string
	Value         *big.Int
	RawValue      string
	MethodIndex   uint64
	PrefillMode   bool
	PrefillStr    string
	PrefillParams []string
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

	CustomABI      string
	JSONOutputFile string
)
