package common

import (
	"encoding/json"
	"math/big"
	"os"
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

// ParamResult is the general struct that aims to be able to store all of the information of a parameter
//  1. Para meter is an arbitrary type such as string, int, uint, bool, address, hash, bytes, fixed bytes
//     ParamResult{
//     Name: "param1", // in case it is an element of an array, name = array_name[index]
//     Type: "string", // or "int", "uint", "bool", "address", "hash", "bytes", "fixed bytes"
//     Values: [1]Value{}, // where this has only one value
//     }
//  2. Parameter is a slice or an array of arbitrary types
//     ParamResult{
//     Name: "param1",
//     Type: "string[]", // or "int[]", "uint[]", "bool[]", "address[]", "hash[]", "bytes[]", "fixed bytes[]"
//     Values: [n]Value{}, // where this has multiple values
//     }
//  3. Parameter is a tuple
//     ParamResult{
//     Name: "param1",
//     Type: "tuple",
//     Tuples: [1][]ParamResult{}, // where this has
//     }
//  4. Parameter is a slice or an array of tuples
//     ParamResult{
//     Name: "param1",
//     Type: "tuple[]", // or "tuple[2]", "tuple[2][3]"
//     Tuples: [n][]ParamResult{}, // where this has multiple tuples, []ParamResult represents a tuple
//     }
//  5. Parameter is a slice or an array of another slice/array
//     ParamResult{
//     Name: "param1",
//     Type: "string[][]", // or "int[][]", "uint[][]", "bool[][]", "address[][]", "hash[][]", "bytes[][]", "fixed bytes[][]"
//     Arrays: [n]ParamResult{}, // where this has multiple arrays, []ParamResult represents an array
//     }
type ParamResult struct {
	Name   string
	Type   string
	Values []Value            // Values stores the values of the parameters, in case the param is an array of arbitrary types, it will have more than one value
	Tuples []TupleParamResult // []ParamResult represents a tuple, this has more than one tuple if the param is a slice or an array of tuples
	Arrays []ParamResult      // Arrays stores the values of the parameters, in case the param is a slice or an array of another slice/array, it will have more than one value
}

type TupleParamResult struct {
	Name   string
	Type   string
	Values []ParamResult
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

func (tr *TxResults) Write(filepath string) error {
	data, _ := json.MarshalIndent(tr, "", "  ")
	return os.WriteFile(filepath, data, 0644)
}

type TxResult struct {
	Hash      string
	Network   string
	Status    string
	From      Address
	Value     string
	To        Address
	Nonce     string
	GasPrice  string
	GasLimit  string
	GasUsed   string
	GasCost   string
	Timestamp string
	TxType    string

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
