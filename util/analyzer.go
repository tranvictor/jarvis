package util

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/tranvictor/ethutils"
	. "github.com/tranvictor/jarvis/common"
)

type TxAnalyzer interface {
	AnalyzeMethodCall(abi *abi.ABI, data []byte, customABIs map[string]*abi.ABI) (method string, params []ParamResult, gnosisResult *GnosisResult, err error)
	AnalyzeOffline(txinfo *ethutils.TxInfo, a *abi.ABI, customABIs map[string]*abi.ABI, isContract bool, network string) *TxResult
	ParamAsJarvisValues(t abi.Type, value interface{}) []Value
}
