package util

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	. "github.com/tranvictor/jarvis/common"
	. "github.com/tranvictor/jarvis/networks"
)

type TxAnalyzer interface {
	AnalyzeFunctionCallRecursively(lookupABI ABIDatabase, value *big.Int, destination string, data []byte, customABIs map[string]*abi.ABI) (fc *FunctionCall)
	AnalyzeMethodCall(a *abi.ABI, data []byte) (method string, params []ParamResult, err error)
	AnalyzeOffline(txinfo *TxInfo, lookupABI ABIDatabase, customABIs map[string]*abi.ABI, isContract bool, network Network) *TxResult
	ParamAsJarvisValues(t abi.Type, value interface{}) []Value
}
