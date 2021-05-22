package common

import (
	"fmt"
	"io"
	"os"
	"strings"

	aurora "github.com/logrusorgru/aurora"
	indent "github.com/openconfig/goyang/pkg/indent"
	"github.com/tranvictor/ethutils"
)

func PrintFunctionCall(fc *FunctionCall) {
	printFunctionCallToWriter(fc, os.Stdout, 0)
}

func PrintTxDetails(result *TxResult, writer io.Writer) {
	fmt.Fprintf(writer, "Tx hash: %s\n", result.Hash)
	if result.Status == "done" {
		fmt.Fprintf(writer, "Mining status: %s\n", aurora.Green(result.Status))
	} else {
		fmt.Fprintf(writer, "Mining status: %s\n", aurora.Bold(aurora.Red(result.Status)))
	}
	fmt.Fprintf(writer, "From: %s\n", VerboseAddress(result.From))
	fmt.Fprintf(writer, "Value: %s ETH\n", result.Value)
	fmt.Fprintf(writer, "To: %s\n", VerboseAddress(result.To))
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

	printFunctionCallToWriter(result.FunctionCall, writer, 0)

	fmt.Fprintf(writer, "\nEvent logs:\n")
	for i, l := range result.Logs {
		fmt.Fprintf(writer, "Log %d: %s\n", i+1, l.Name)
		for j, topic := range l.Topics {
			fmt.Fprintf(writer, "    Topic %d - %s: %s\n", j+1, topic.Name, DisplayValues(topic.Value))
		}
		fmt.Fprintf(writer, "    Data:\n")
		for _, param := range l.Data {
			fmt.Fprintf(writer, "    %s (%s): %s\n", param.Name, param.Type, DisplayValues(param.Value))
		}
	}
}

func printFunctionCallToWriter(fc *FunctionCall, w io.Writer, level int) {
	indentation := ""
	for i := 0; i < level; i++ {
		indentation = indentation + "    "
	}
	writer := indent.NewWriter(w, indentation)

	if fc.Method == "" {
		fmt.Fprintf(writer, "Getting ABI and function name failed: %s\n", fc.Error)
		return
	}

	if level > 0 {
		fmt.Fprintf(writer, "Interpreted Contract call: %s\n", VerboseAddress(fc.Destination))
		fmt.Fprintf(writer, "| Value: %f ETH\n", ethutils.BigToFloat(fc.Value, 18))
	} else {
		fmt.Fprintf(writer, "Contract: %s\n", VerboseAddress(fc.Destination))
	}
	fmt.Fprintf(writer, "| Method: %s\n", fc.Method)
	fmt.Fprintf(writer, "| Params:\n")
	for _, param := range fc.Params {
		fmt.Fprintf(writer, "|    %s (%s): %s\n", param.Name, param.Type, DisplayValues(param.Value))
	}
	for _, dfc := range fc.DecodedFunctionCalls {
		printFunctionCallToWriter(dfc, w, level+1)
	}
}

func ReadableNumber(value string) string {
	digits := []string{}
	for i, _ := range value {
		digits = append([]string{string(value[len(value)-1-i])}, digits...)
		if (i+1)%3 == 0 && i < len(value)-1 {
			if (i+1)%9 == 0 {
				digits = append([]string{"‸"}, digits...)
			} else {
				digits = append([]string{"￺"}, digits...)
			}
		}
	}
	return fmt.Sprintf("%s (%s)", value, strings.Join(digits, ""))
}

func verboseValue(value Value) string {
	if value.Address != nil {
		return VerboseAddress(*value.Address)
	}

	if value.Type == "string" {
		return value.Value
	}

	// if this is a hex, it is likely to be a byte data so don't display
	// in readable number
	if len(value.Value) >= 2 && value.Value[0:2] == "0x" {
		return value.Value
	}
	// otherwise, it is a number then return it in a readable format
	return ReadableNumber(value.Value)
}

func VerboseValues(values []Value) []string {
	result := []string{}
	for _, value := range values {
		result = append(result, verboseValue(value))
	}
	return result
}

func DisplayValues(values []Value) string {
	verboseValues := VerboseValues(values)
	if len(verboseValues) == 0 {
		return ""
	} else if len(verboseValues) == 1 {
		return verboseValues[0]
	} else {
		parts := []string{}
		for i, value := range values {
			parts = append(parts, fmt.Sprintf("%d. %s", i+1, verboseValue(value)))
		}
		return strings.Join(parts, "\n")
	}
}

func VerboseAddress(addr Address) string {
	if addr.Decimal != 0 {
		return fmt.Sprintf("%s (%s)", addr.Address, NameWithColor(fmt.Sprintf("%s - %d", addr.Desc, addr.Decimal)))
	}
	return fmt.Sprintf("%s (%s)", addr.Address, NameWithColor(addr.Desc))
}
