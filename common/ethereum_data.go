package common

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

type Address struct {
	Address string
	Desc    string
	Decimal int64
}

type Value struct {
	Value   string
	Type    string
	Address *Address
}

type ParamResult struct {
	Name  string
	Type  string
	Value []Value
}

type TopicResult struct {
	Name  string
	Value []Value
}

type LogResult struct {
	Name   string
	Topics []TopicResult
	Data   []ParamResult
}

type GnosisResult struct {
	Contract   Address
	Network    string
	Method     string
	Params     []ParamResult
	GnosisInit *GnosisResult
	Error      string
}

type TxResults map[string]*TxResult

func (tr *TxResults) Write(filepath string) {
	data, _ := json.MarshalIndent(tr, "", "  ")
	err := ioutil.WriteFile(filepath, data, 0644)
	if err != nil {
		fmt.Printf("Writing to json file failed: %s\n", err)
	}
}

type TxResult struct {
	Hash     string
	Network  string
	Status   string
	From     Address
	Value    string
	To       Address
	Nonce    string
	GasPrice string
	GasLimit string
	GasUsed  string
	GasCost  string
	TxType   string

	Contract Address
	Method   string
	Params   []ParamResult
	Logs     []LogResult

	GnosisInit *GnosisResult

	Completed bool
	Error     string
}

func NewTxResult() *TxResult {
	return &TxResult{
		Hash:       "",
		Network:    "mainnet",
		Status:     "",
		From:       Address{},
		Value:      "",
		To:         Address{},
		Nonce:      "",
		GasPrice:   "",
		GasLimit:   "",
		GasUsed:    "",
		GasCost:    "",
		TxType:     "",
		Contract:   Address{},
		Method:     "",
		Params:     []ParamResult{},
		Logs:       []LogResult{},
		GnosisInit: nil,
		Completed:  false,
		Error:      "",
	}
}
