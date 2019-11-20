package tx

import (
	"fmt"
	"io"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi"
	. "github.com/logrusorgru/aurora"
	"github.com/tranvictor/ethutils/txanalyzer"
)

var analyzer *txanalyzer.TxAnalyzer
var ropstenAnalyzer *txanalyzer.TxAnalyzer

func getAnalyzer(network string) *txanalyzer.TxAnalyzer {
	switch network {
	case "ropsten":
		if ropstenAnalyzer == nil {
			ropstenAnalyzer = txanalyzer.NewRopstenAnalyzer()
		}
		return ropstenAnalyzer
	case "mainnet":
		if analyzer == nil {
			analyzer = txanalyzer.NewAnalyzer()
		}
		return analyzer
	case "tomo":
		if analyzer == nil {
			analyzer = txanalyzer.NewTomoAnalyzer()
		}
		return analyzer
	}
	return nil
}

func AnalyzeMethodCallAndPrint(abi *abi.ABI, data []byte, network string) {
	analyzer := getAnalyzer(network)

	methodName, params, gnosisResult, err := analyzer.AnalyzeMethodCall(abi, data)
	if err != nil {
		fmt.Printf("Couldn't analyze method call: %s\n", err)
		return
	}
	fmt.Printf("  Method: %s\n", methodName)
	fmt.Printf("  Params:\n")
	for _, param := range params {
		fmt.Printf("    %s (%s): %s\n", param.Name, param.Type, param.Value)
	}
	if gnosisResult != nil {
		PrintGnosis(gnosisResult)
	}
}

func AnalyzeAndPrint(tx string, network string) {
	analyzer := getAnalyzer(network)
	result := analyzer.Analyze(tx)
	printToStdout(result, os.Stdout)
}

func nameWithColor(name string) string {
	if name == "unknown" {
		return Red(name).String()
	} else {
		return Green(name).String()
	}
}

func PrintGnosis(result *txanalyzer.GnosisResult) {
	printGnosisToWriter(result, os.Stdout)
}

func printGnosisToWriter(result *txanalyzer.GnosisResult, writer io.Writer) {
	if result != nil {
		fmt.Fprintf(writer, "Gnosis multisig init data:\n")
		if result.Method == "" {
			fmt.Fprintf(writer, "Couldn't decode gnosis call method\n")
			return
		}
		fmt.Fprintf(writer, "Contract: %s - %s\n", result.Contract.Address, nameWithColor(result.Contract.Name))
		fmt.Fprintf(writer, "Method: %s\n", result.Method)
		fmt.Fprintf(writer, "Params:\n")
		for _, param := range result.Params {
			fmt.Fprintf(writer, "    %s (%s): %s\n", param.Name, param.Type, param.Value)
		}
	}
}

func printToStdout(result *txanalyzer.TxResult, writer io.Writer) {
	fmt.Fprintf(writer, "Tx hash: %s\n", result.Hash)
	if result.Status == "done" {
		fmt.Fprintf(writer, "Mining status: %s\n", Green(result.Status))
	} else {
		fmt.Fprintf(writer, "Mining status: %s\n", Bold(Red(result.Status)))
	}
	fmt.Fprintf(writer, "From: %s - (%s)\n", result.From.Address, nameWithColor(result.From.Name))
	fmt.Fprintf(writer, "Value: %s ETH\n", result.Value)
	fmt.Fprintf(writer, "To: %s - (%s)\n", result.To.Address, nameWithColor(result.To.Name))
	fmt.Fprintf(writer, "Nonce: %s\n", result.Nonce)
	fmt.Fprintf(writer, "Gas price: %s gwei\n", result.GasPrice)
	fmt.Fprintf(writer, "Gas limit: %s\n", result.GasLimit)

	if result.TxType == "" {
		fmt.Fprintf(writer, "Checking tx type failed: %s\n", result.Error)
		return
	}

	fmt.Fprintf(writer, "Tx type: %s\n", result.TxType)
	if result.TxType == "normal" {
		return
	}

	if result.Method == "" {
		fmt.Fprintf(writer, "Getting ABI and function name failed: %s\n", result.Error)
		return
	}
	fmt.Fprintf(writer, "Contract: %s - (%s)\n", result.Contract.Address, nameWithColor(result.Contract.Name))
	fmt.Fprintf(writer, "Method: %s\n", result.Method)
	fmt.Fprintf(writer, "Params:\n")
	for _, param := range result.Params {
		fmt.Fprintf(writer, "    %s (%s): %s\n", param.Name, param.Type, param.Value)
	}
	fmt.Fprintf(writer, "Event logs:\n")
	for i, l := range result.Logs {
		fmt.Fprintf(writer, "Log %d: %s\n", i+1, l.Name)
		for j, topic := range l.Topics {
			fmt.Fprintf(writer, "    Topic %d - %s: %s\n", j+1, topic.Name, topic.Value)
		}
		fmt.Fprintf(writer, "    Data:\n")
		for _, param := range l.Data {
			fmt.Fprintf(writer, "    %s (%s): %s\n", param.Name, param.Type, param.Value)
		}
	}
	printGnosisToWriter(result.GnosisInit, writer)
	// if result.Error != "" {
	// 	fmt.Fprintf(writer, "Error during tx analysis: %s\n", result.Error)
	// }
}
