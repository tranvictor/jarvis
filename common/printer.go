package common

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/tranvictor/jarvis/config"
)

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

// VerboseValue returns a human-readable string for a single decoded ABI value.
// Addresses are expanded with their label; token amounts show both raw and
// human-readable form; other numbers are formatted with digit separators.
func VerboseValue(value Value) string {
	if value.Address != nil {
		return VerboseAddress(*value.Address)
	}

	if value.Type == "string" {
		return value.Value
	}

	// hex values are likely byte data — skip numeric formatting
	if len(value.Value) >= 2 && value.Value[0:2] == "0x" {
		return value.Value
	}

	if value.TokenHint != nil {
		raw := StringToBig(value.Value)
		human := BigToFloatString(raw, value.TokenHint.Decimal)
		if value.TokenHint.Symbol != "" {
			return fmt.Sprintf("%s (%s %s)", value.Value, human, value.TokenHint.Symbol)
		}
		return fmt.Sprintf("%s (%s)", value.Value, human)
	}

	return ReadableNumber(value.Value)
}

// VerboseValues returns VerboseValue for each element in a slice.
func VerboseValues(values []Value) []string {
	result := []string{}
	for _, value := range values {
		result = append(result, VerboseValue(value))
	}
	return result
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
			DebugObjPrint(v.Index(i).Interface())
		}
	default:
		fmt.Printf("Type: %s, Value: %v\n", v.Type(), v.Interface())
	}
}
