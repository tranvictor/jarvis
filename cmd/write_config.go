package cmd

import (
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/accounts"
)

var (
	GasPrice             float64
	ExtraGasPrice        float64
	GasLimit             uint64
	ExtraGasLimit        uint64
	Nonce                uint64
	From                 string
	FromAcc              accounts.AccDesc
	To                   string
	Value                float64
	MethodIndex          uint64
	PrefillMode          bool
	PrefillStr           string
	PrefillParams        []string
	Tx                   string
	TxInfo               *ethutils.TxInfo
	AllZeroParamsMethods bool
)
