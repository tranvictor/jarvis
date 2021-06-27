package common

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
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

type FunctionCall struct {
	Destination          Address
	Value                *big.Int
	Method               string
	Params               []ParamResult
	DecodedFunctionCalls []*FunctionCall
	Error                string
}

type ParamResult struct {
	Name  string
	Type  string
	Value []Value
	Tuple []ParamResult
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

	FunctionCall *FunctionCall
	Logs         []LogResult

	Completed bool
	Error     string
}

func NewTxResult() *TxResult {
	return &TxResult{
		Hash:         "",
		Network:      "mainnet",
		Status:       "",
		From:         Address{},
		Value:        "",
		To:           Address{},
		Nonce:        "",
		GasPrice:     "",
		GasLimit:     "",
		GasUsed:      "",
		GasCost:      "",
		TxType:       "",
		FunctionCall: &FunctionCall{},
		Logs:         []LogResult{},
		Completed:    false,
		Error:        "",
	}
}
