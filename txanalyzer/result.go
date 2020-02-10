package txanalyzer

import (
	"fmt"
	"io"
)

type AddressResult struct {
	Address string
	Name    string
}

type ParamResult struct {
	Name  string
	Type  string
	Value string
}

type TopicResult struct {
	Name  string
	Value string
}

type LogResult struct {
	Name   string
	Topics []TopicResult
	Data   []ParamResult
}

type GnosisResult struct {
	Contract AddressResult
	Method   string
	Params   []ParamResult
	Error    string
}

type TxResult struct {
	Hash     string
	Status   string
	From     AddressResult
	Value    string
	To       AddressResult
	Nonce    string
	GasPrice string
	GasLimit string
	TxType   string

	Contract AddressResult
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
		Status:     "",
		From:       AddressResult{},
		Value:      "",
		To:         AddressResult{},
		Nonce:      "",
		GasPrice:   "",
		GasLimit:   "",
		TxType:     "",
		Contract:   AddressResult{},
		Method:     "",
		Params:     []ParamResult{},
		Logs:       []LogResult{},
		GnosisInit: nil,
		Completed:  false,
		Error:      "",
	}
}

func (self *TxResult) Print(writer io.Writer) {
	fmt.Fprintf(writer, "Tx hash: %s\n", self.Hash)
	fmt.Fprintf(writer, "Mining status: %s\n", self.Status)
	fmt.Fprintf(writer, "From: %s - (%s)\n", self.From.Address, self.From.Name)
	fmt.Fprintf(writer, "Value: %s ETH\n", self.Value)
	fmt.Fprintf(writer, "To: %s - (%s)\n", self.To.Address, self.To.Name)
	fmt.Fprintf(writer, "Nonce: %s\n", self.Nonce)
	fmt.Fprintf(writer, "Gas price: %s gwei\n", self.GasPrice)
	fmt.Fprintf(writer, "Gas limit: %s\n", self.GasLimit)

	if self.TxType == "" {
		fmt.Fprintf(writer, "Checking tx type failed: %s\n", self.Error)
		return
	}

	fmt.Fprintf(writer, "Tx type: %s\n", self.TxType)
	if self.TxType == "normal" {
		return
	}

	if self.Method == "" {
		fmt.Fprintf(writer, "Getting ABI and function name failed: %s\n", self.Error)
		return
	}
	fmt.Fprintf(writer, "Contract: %s - (%s)\n", self.Contract.Address, self.Contract.Name)
	fmt.Fprintf(writer, "Method: %s\n", self.Method)
	fmt.Fprintf(writer, "Params:\n")
	for _, param := range self.Params {
		fmt.Fprintf(writer, "    %s (%s): %s\n", param.Name, param.Type, param.Value)
	}
	fmt.Fprintf(writer, "Event logs:\n")
	for i, l := range self.Logs {
		fmt.Fprintf(writer, "Log %d: %s\n", i+1, l.Name)
		for j, topic := range l.Topics {
			fmt.Fprintf(writer, "    Topic %d - %s: %s\n", j+1, topic.Name, topic.Value)
		}
		fmt.Fprintf(writer, "    Data:\n")
		for _, param := range l.Data {
			fmt.Fprintf(writer, "    %s (%s): %s\n", param.Name, param.Type, param.Value)
		}
	}
	if self.GnosisInit != nil {
		fmt.Fprintf(writer, "Gnosis multisig init data:\n")
		if self.GnosisInit.Method == "" {
			fmt.Fprintf(writer, "Getting ABI and function name failed: %s\n", self.Error)
			return
		}
		fmt.Fprintf(writer, "Contract: %s - %s\n", self.GnosisInit.Contract.Address, self.GnosisInit.Contract.Name)
		fmt.Fprintf(writer, "Method: %s\n", self.GnosisInit.Method)
		fmt.Fprintf(writer, "Params:\n")
		for _, param := range self.GnosisInit.Params {
			fmt.Fprintf(writer, "    %s (%s): %s\n", param.Name, param.Type, param.Value)
		}
	}
	// if self.Error != "" {
	// 	fmt.Fprintf(writer, "Error during tx analysis: %s\n", self.Error)
	// }
}
