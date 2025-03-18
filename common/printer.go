package common

import (
	"fmt"
	"io"
	"os"
	"reflect"
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
			PrintVerboseTopicToWriter(writer, topic)
		}
		fmt.Fprintf(writer, "    Data:\n")
		for _, param := range l.Data {
			PrintVerboseParamResultToWriter(writer, param, 1, true)
		}
	}
}

func PrintVerboseTopicToWriter(writer io.Writer, topic TopicResult) {
	PrintVerboseValueToWriter(writer, 0, topic.Value)
	fmt.Fprintf(writer, "\n")
}

func PrintVerboseParamResultToWriter(writer io.Writer, param ParamResult, indentLevel int, includeFieldNames bool) {
	indentation := ""
	for range indentLevel {
		indentation = indentation + "    "
	}

	var fieldStr string
	if includeFieldNames {
		fieldStr = fmt.Sprintf("%s%s (%s): ", indentation, param.Name, param.Type)
	} else {
		fieldStr = indentation
	}

	fmt.Fprintf(writer, "%s", fieldStr)

	// in case the param is an array of values
	if param.Values != nil {
		PrintVerboseValueToWriter(writer, len(fieldStr), param.Values)
		return
	}
	// in case the param is an array of tubples
	if param.Tuples != nil {
		PrintVerboseTuplesToWriter(writer, param.Tuples, indentLevel)
		return
	}
	// in case the param is an array of another array
	if param.Arrays != nil {
		PrintVerboseArraysToWriter(writer, param.Arrays, 0)
		return
	}
}

func PrintVerboseTuplesToWriter(writer io.Writer, tuples []TupleParamResult, indentLevel int) {
	indentation := ""
	for range indentLevel {
		indentation = indentation + "    "
	}

	if len(tuples) == 0 {
		fmt.Fprintf(writer, "\n")
		return
	}

	if len(tuples) == 1 {
		PrintVerboseTupleToWriter(writer, tuples[0], indentLevel, false)
		return
	}

	// print the first tuple's field signature
	// PrintTupleFieldNamesToWriter(writer, tuples[0], indentLevel)
	fmt.Fprintf(writer, "[")
	for i, tuple := range tuples {
		PrintVerboseTupleToWriter(writer, tuple, indentLevel, false)
		if i < len(tuples)-1 {
			fmt.Fprintf(writer, ", ")
		}
	}
	fmt.Fprintf(writer, "]")
}

func PrintTupleFieldNamesToWriter(writer io.Writer, tuple TupleParamResult, indentLevel int) {
	indentation := ""
	for i := 0; i < indentLevel; i++ {
		indentation = indentation + "    "
	}
	fmt.Fprintf(writer, "(")
	for i, field := range tuple.Values {
		fmt.Fprintf(writer, "%s", field.Name)
		if i < len(tuple.Values)-1 {
			fmt.Fprintf(writer, ", ")
		}
	}
	fmt.Fprintf(writer, ")")
}

func PrintVerboseTupleToWriter(writer io.Writer, tuple TupleParamResult, indentLevel int, includeFieldNames bool) {
	indentation := ""
	for range indentLevel {
		indentation = indentation + "    "
	}

	if includeFieldNames {
		fmt.Fprintf(writer, "%s(", tuple.Name)
	} else {
		fmt.Fprintf(writer, "(")
	}

	for i, field := range tuple.Values {
		PrintVerboseParamResultToWriter(writer, field, 0, includeFieldNames)
		if i < len(tuple.Values)-1 {
			fmt.Fprintf(writer, ", ")
		}
	}
	fmt.Fprintf(writer, ")")
}

func PrintVerboseArraysToWriter(writer io.Writer, arrays []ParamResult, indentLevel int) {
	indentation := ""
	for range indentLevel {
		indentation = indentation + "    "
	}

	fmt.Fprintf(writer, "%s[", indentation)
	for i, array := range arrays {
		PrintVerboseParamResultToWriter(writer, array, 0, true)
		if i < len(arrays)-1 {
			fmt.Fprintf(writer, ", ")
		}
	}
	fmt.Fprintf(writer, "]")
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
			PrintVerboseValueToWriter(writer, 0, topic.Value)
			fmt.Fprintf(writer, "\n")
		}
		fmt.Fprintf(writer, "    Data:\n")
		for _, param := range l.Data {
			PrintVerboseParamResultToWriter(writer, param, 1, true)
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
		PrintVerboseParamResultToWriter(writer, param, 1, true)
		fmt.Fprintf(writer, "\n")
	}
	for _, dfc := range fc.DecodedFunctionCalls {
		printFunctionCallToWriter(dfc, w, level+1)
	}
}

func ReadableNumber(value string) string {
	if len(value) <= 4 {
		return value
	}

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

func PrintVerboseValueToWriter(writer io.Writer, leftAlign int, values []Value) {
	anchor := ""
	for i := 0; i < leftAlign; i++ {
		anchor = anchor + " "
	}

	verboseValues := VerboseValues(values)
	if len(verboseValues) == 0 {
		fmt.Fprintf(writer, "")
	} else if len(verboseValues) == 1 {
		fmt.Fprintf(writer, "%s", verboseValues[0])
	} else {
		for i, value := range values {
			if i > 0 {
				fmt.Fprintf(writer, anchor)
			}
			fmt.Fprintf(writer, "%d. %s", i+1, verboseValue(value))
			if i < len(values)-1 {
				fmt.Fprintf(writer, "\n")
			}
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
		return fmt.Printf(format, a...)
	}

	return 0, nil
}

func DebugObjPrint(obj interface{}) {
	if !config.Debug {
		return
	}
	v := reflect.ValueOf(obj)
	// Handle the case where obj might be a pointer
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		fmt.Println("Struct fields and tags:")
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			value := v.Field(i)
			fmt.Printf("Field: %-10s Value: %-10v Tag: '%s'\n", field.Name, value, field.Tag)
		}
	case reflect.Slice, reflect.Array:
		fmt.Printf("Slice or Array of %s:\n", v.Type().Elem())
		maxElements := v.Len()
		if maxElements > 10 {
			maxElements = 10
		}
		for i := 0; i < maxElements; i++ {
			fmt.Printf("Element %d: ", i)
			DebugObjPrint(v.Index(i).Interface()) // Recursively print elements
		}
	default:
		// Handle basic non-struct types
		fmt.Printf("Type: %s, Value: %v\n", v.Type(), v.Interface())
	}
}
