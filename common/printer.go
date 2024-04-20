package common

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	aurora "github.com/logrusorgru/aurora"
	indent "github.com/openconfig/goyang/pkg/indent"

	"github.com/tranvictor/jarvis/config"
	. "github.com/tranvictor/jarvis/networks"
)

func PrintFunctionCall(fc *FunctionCall) {
	printFunctionCallToWriter(fc, os.Stdout, 0)
}

func PrintTxSuccessSummary(result *TxResult, network Network, writer io.Writer) {
	if result.Status == "done" {
		fmt.Fprintf(writer, "Mining status: %s\n", aurora.Green(result.Status))
	} else {
		fmt.Fprintf(writer, "Mining status: %s\n", aurora.Bold(aurora.Red(result.Status)))
	}

	fmt.Fprintf(
		writer, "From: %s ===[%s %s]===> %s\n",
		VerboseAddress(result.From),
		result.Value, network.GetNativeTokenSymbol(),
		VerboseAddress(result.To),
	)

	fmt.Fprintf(writer, "\nEvent logs:\n")
	for i, l := range result.Logs {
		fmt.Fprintf(writer, "Log %d: %s\n", i+1, l.Name)
		for j, topic := range l.Topics {
			fmt.Fprintf(writer, "    Topic %d - %s: ", j+1, topic.Name)
			PrintVerboseValueToWriter(writer, topic.Value)
		}
		fmt.Fprintf(writer, "    Data:\n")
		for _, param := range l.Data {
			fmt.Fprintf(writer, "    %s (%s): ", param.Name, param.Type)
			PrintVerboseValueToWriter(writer, param.Value)
		}
	}
}

func PrintTxDetails(result *TxResult, network Network, writer io.Writer) {
	if result.Status == "done" {
		fmt.Fprintf(writer, "Mining status: %s\n", aurora.Green(result.Status))
	} else {
		fmt.Fprintf(writer, "Mining status: %s\n", aurora.Bold(aurora.Red(result.Status)))
	}

	fmt.Fprintf(
		writer, "From: %s ===[%s %s]===> %s\n",
		VerboseAddress(result.From),
		result.Value, network.GetNativeTokenSymbol(),
		VerboseAddress(result.To),
	)
	fmt.Fprintf(writer, "Nonce: %s\n", result.Nonce)
	fmt.Fprintf(writer, "Gas price: %s gwei\n", result.GasPrice)
	fmt.Fprintf(writer, "Gas limit: %s\n", result.GasLimit)

	if result.TxType == "" {
		fmt.Fprintf(writer, "Checking tx type failed: %s\n", result.Error)
		return
	}

	if result.TxType == "normal" {
		return
	}

	printFunctionCallToWriter(result.FunctionCall, writer, 0)

	fmt.Fprintf(writer, "\nEvent logs:\n")
	for i, l := range result.Logs {
		fmt.Fprintf(writer, "Log %d: %s\n", i+1, l.Name)
		for j, topic := range l.Topics {
			fmt.Fprintf(writer, "    Topic %d - %s: ", j+1, topic.Name)
			PrintVerboseValueToWriter(writer, topic.Value)
		}
		fmt.Fprintf(writer, "    Data:\n")
		for _, param := range l.Data {
			fmt.Fprintf(writer, "    %s (%s): ", param.Name, param.Type)
			PrintVerboseValueToWriter(writer, param.Value)
		}
	}
}

func printParamToWriter(p ParamResult, w io.Writer, parentw io.Writer, level int) {
	indentation := ""
	for i := 0; i < level; i++ {
		indentation = indentation + "    "
	}
	writer := indent.NewWriter(w, indentation)

	fmt.Fprintf(writer, "    %s (%s): ", p.Name, p.Type)
	PrintVerboseValueToWriter(writer, p.Value)
	if len(p.Tuple) > 0 {
		for _, f := range p.Tuple {
			printParamToWriter(f, writer, parentw, level+1)
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
		fmt.Fprintf(writer, "Interpreted Contract call to: %s\n", VerboseAddress(fc.Destination))
		fmt.Fprintf(writer, "| Value: %f ETH\n", BigToFloat(fc.Value, 18))
	} else {
		fmt.Fprintf(writer, "\n")
	}
	fmt.Fprintf(writer, "| Method: %s\n", fc.Method)
	fmt.Fprintf(writer, "| Params:\n")
	for _, param := range fc.Params {
		printParamToWriter(param, writer, w, 0)
	}
	for _, dfc := range fc.DecodedFunctionCalls {
		printFunctionCallToWriter(dfc, w, level+1)
	}
}

func ReadableNumber(value string) string {
	digits := []string{}
	for i := range value {
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

func PrintVerboseValueToWriter(writer io.Writer, values []Value) {
	verboseValues := VerboseValues(values)
	if len(verboseValues) == 0 {
		fmt.Fprintf(writer, "\n")
	} else if len(verboseValues) == 1 {
		fmt.Fprintf(writer, "%s\n", verboseValues[0])
	} else {
		fmt.Fprintf(writer, "\n")
		for i, value := range values {
			fmt.Fprintf(writer, "        %d. %s\n", i+1, verboseValue(value))
		}
	}
}

func VerboseAddress(addr Address) string {
	if addr.Decimal != 0 {
		return fmt.Sprintf(
			"%s (%s)",
			addr.Address,
			NameWithColor(fmt.Sprintf("%s - %d", addr.Desc, addr.Decimal)),
		)
	}
	return fmt.Sprintf("%s (%s)", addr.Address, NameWithColor(addr.Desc))
}

func PrintElapseTime(start time.Time, str string) {
	DebugPrintf(
		"-------------------------------------profiling-elapsed: %s -- %s\n",
		time.Since(start),
		str,
	)
}

func DebugPrintf(format string, a ...any) (n int, err error) {
	if config.Debug {
		return fmt.Printf(format, a)
	}

	return 0, nil
}
