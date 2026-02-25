package util

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

type TxAnalyzer interface {
	AnalyzeFunctionCallRecursively(lookupABI jarviscommon.ABIDatabase, value *big.Int, destination string, data []byte, customABIs map[string]*abi.ABI) (fc *jarviscommon.FunctionCall)
	AnalyzeMethodCall(a *abi.ABI, data []byte) (method string, params []jarviscommon.ParamResult, err error)
	AnalyzeOffline(txinfo *jarviscommon.TxInfo, lookupABI jarviscommon.ABIDatabase, customABIs map[string]*abi.ABI, isContract bool) *jarviscommon.TxResult
	ParamAsJarvisParamResult(name string, t abi.Type, value interface{}) jarviscommon.ParamResult
}
