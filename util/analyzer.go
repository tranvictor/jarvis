package util

import (
	"fmt"
	"io"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi"
	. "github.com/logrusorgru/aurora"
	"github.com/tranvictor/jarvis/txanalyzer"
)

func AnalyzeMethodCallAndPrint(analyzer *txanalyzer.TxAnalyzer, abi *abi.ABI, data []byte, network string) {
	methodName, params, gnosisResult, err := analyzer.AnalyzeMethodCall(abi, data)
	if err != nil {
		fmt.Printf("Couldn't analyze method call: %s\n", err)
		return
	}
	fmt.Printf("  Method: %s\n", methodName)
	fmt.Printf("  Params:\n")
	for _, param := range params {
		fmt.Printf("    %s (%s): %s\n", param.Name, param.Type, DisplayValues(param.Value, analyzer.Network))
	}
	if gnosisResult != nil {
		PrintGnosis(gnosisResult)
	}
}

func AnalyzeAndPrint(analyzer *txanalyzer.TxAnalyzer, tx string, network string) {
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
		fmt.Fprintf(writer, "\n     __________________________")
		fmt.Fprintf(writer, "\n     Gnosis multisig init data: ")
		if result.Method == "" {
			fmt.Fprintf(writer, "Couldn't decode gnosis call method\n")
			return
		}
		fmt.Fprintf(writer, "\n     Contract: %s\n", VerboseAddress(result.Contract, result.Network))
		fmt.Fprintf(writer, "     Method: %s\n", result.Method)
		fmt.Fprintf(writer, "     Params:\n")
		for _, param := range result.Params {
			fmt.Fprintf(writer, "       %s (%s): %s\n", param.Name, param.Type, DisplayValues(param.Value, result.Network))
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
	fmt.Fprintf(writer, "From: %s\n", VerboseAddress(result.From, result.Network))
	fmt.Fprintf(writer, "Value: %s ETH\n", result.Value)
	fmt.Fprintf(writer, "To: %s\n", VerboseAddress(result.To, result.Network))
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
	fmt.Fprintf(writer, "\nContract: %s\n", VerboseAddress(result.Contract, result.Network))
	fmt.Fprintf(writer, "Method: %s\n", result.Method)
	fmt.Fprintf(writer, "Params:\n")
	for _, param := range result.Params {
		fmt.Fprintf(writer, "    %s (%s): %s\n", param.Name, param.Type, DisplayValues(param.Value, result.Network))
	}
	printGnosisToWriter(result.GnosisInit, writer)
	fmt.Fprintf(writer, "\nEvent logs:\n")
	for i, l := range result.Logs {
		fmt.Fprintf(writer, "Log %d: %s\n", i+1, l.Name)
		for j, topic := range l.Topics {
			fmt.Fprintf(writer, "    Topic %d - %s: %s\n", j+1, topic.Name, DisplayValues(topic.Value, result.Network))
		}
		fmt.Fprintf(writer, "    Data:\n")
		for _, param := range l.Data {
			fmt.Fprintf(writer, "    %s (%s): %s\n", param.Name, param.Type, DisplayValues(param.Value, result.Network))
		}
	}
}
