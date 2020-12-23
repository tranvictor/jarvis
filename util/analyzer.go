package util

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/tranvictor/ethutils"
	. "github.com/tranvictor/jarvis/common"
)

type TxAnalyzer interface {
	AnalyzeMethodCall(abi *abi.ABI, data []byte) (method string, params []ParamResult, gnosisResult *GnosisResult, err error)
	AnalyzeOffline(txinfo *ethutils.TxInfo, abi *abi.ABI, isContract bool) *TxResult
}
