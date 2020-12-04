package util

import (
	"fmt"
	"io"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi"
	. "github.com/logrusorgru/aurora"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/ethutils/reader"
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

func AnalyzeAndPrint(
	reader *reader.EthReader,
	analyzer *txanalyzer.TxAnalyzer,
	tx string,
	network string,
	forceERC20ABI bool,
	customABI string) {

	txinfo, err := reader.TxInfoFromHash(tx)
	if err != nil {
		fmt.Printf("getting tx info failed: %s", err)
		return
	}

	contractAddress := txinfo.Tx.To().Hex()

	code, err := reader.GetCode(contractAddress)
	if err != nil {
		fmt.Printf("checking tx type failed: %s", err)
		return
	}
	isContract := len(code) > 0

	var result *txanalyzer.TxResult

	if isContract {
		var a *abi.ABI
		var err error

		a, err = GetABI(contractAddress, network)
		if err != nil {
			if forceERC20ABI {
				a, err = ethutils.GetERC20ABI()
			} else if customABI != "" {
				fmt.Printf("%s doesn't have abi on etherscan nor jarvis cache, try custom abi passed in the param\n")
				a, err = ReadCustomABI(contractAddress, customABI, network)
			}
		}
		if err != nil {
			fmt.Printf("Couldn't get the ABI: %s", err)
			return
		}
		result = analyzer.AnalyzeOffline(&txinfo, a, true)
	} else {
		result = analyzer.AnalyzeOffline(&txinfo, nil, false)
	}

	printToStdout(result, os.Stdout)
}

func AlertColor(str string) string {
	return Red(str).String()
}

func InfoColor(str string) string {
	return Green(str).String()
}

func nameWithColor(name string) string {
	if name == "unknown" {
		return AlertColor(name)
	} else {
		return InfoColor(name)
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
		fmt.Fprintf(writer, "Mining status: %s\n", InfoColor(result.Status))
	} else {
		fmt.Fprintf(writer, "Mining status: %s\n", AlertColor(result.Status))
	}
	fmt.Fprintf(
		writer,
		"From: %s ==> %s\n",
		VerboseAddress(result.From, result.Network),
		VerboseAddress(result.To, result.Network),
	)
	fmt.Fprintf(writer, "Value: %s ETH\n", result.Value)
	fmt.Fprintf(
		writer,
		"Nonce: %s  |  Gas: %s gwei (%s gas, %s ETH)\n",
		result.Nonce,
		result.GasPrice,
		result.GasUsed,
		result.GasCost,
	)

	if result.TxType == "" {
		fmt.Fprintf(writer, "Checking tx type failed: %s\n", result.Error)
		return
	}

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
