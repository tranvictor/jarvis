package cmd

import (
	"github.com/tranvictor/jarvis/accounts"
)

var GasPrice float64
var ExtraGasPrice float64
var GasLimit uint64
var ExtraGasLimit uint64
var Nonce uint64
var From string
var FromAcc accounts.AccDesc
var To string
var Value float64
var MethodIndex uint64
var PrefillMode bool
var PrefillStr string
var PrefillParams []string
