package common

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/tranvictor/jarvis/config"
)

// uintMaxSentinels maps decimal representations of common uintN.max values
// to their canonical "infinity" label. Smart contracts widely use these as
// "do the maximum / unlimited / withdraw all" sentinels (Aave withdraw, ERC20
// approve, Uniswap deadline, etc.) and rendering them as the literal 78-digit
// number actively obscures intent. Exact-match only — we never want to round
// values that just happen to be close to the max.
var uintMaxSentinels = map[string]string{
	// 2**256 - 1
	"115792089237316195423570985008687907853269984665640564039457584007913129639935": "uint256.max (∞)",
	// 2**128 - 1
	"340282366920938463463374607431768211455": "uint128.max",
	// 2**64 - 1
	"18446744073709551615": "uint64.max",
	// 2**32 - 1
	"4294967295": "uint32.max",
}

// MaxUintLabel returns the canonical "uintN.max" label and true when value
// matches one of the well-known max-uint sentinels. Empty string and false
// otherwise. Cheap (map lookup) and safe to call on every integer render.
func MaxUintLabel(value string) (string, bool) {
	label, ok := uintMaxSentinels[value]
	return label, ok
}

func ReadableNumber(value string) string {
	if label, ok := MaxUintLabel(value); ok {
		return label
	}
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
func VerboseValue(value Value) string {
	switch value.Kind {
	case DisplayAddress:
		return VerboseAddress(*value.Address)
	case DisplayToken:
		if label, ok := MaxUintLabel(value.Raw); ok {
			if value.Token.Symbol != "" {
				return fmt.Sprintf("%s (%s, all %s)", value.Raw, label, value.Token.Symbol)
			}
			return fmt.Sprintf("%s (%s)", value.Raw, label)
		}
		human := BigToFloatString(StringToBig(value.Raw), value.Token.Decimal)
		if value.Token.Symbol != "" {
			return fmt.Sprintf("%s (%s %s)", value.Raw, human, value.Token.Symbol)
		}
		return fmt.Sprintf("%s (%s)", value.Raw, human)
	case DisplayInteger:
		return ReadableNumber(value.Raw)
	default: // DisplayRaw — string, bool, hash, hex bytes
		return value.Raw
	}
}

// VerboseValues returns VerboseValue for each element in a slice.
func VerboseValues(values []Value) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = VerboseValue(v)
	}
	return out
}

// PlainAddress formats an Address as a plain string with no ANSI color codes.
// Use this when the result will be stored in a data structure or serialized to
// JSON so that consumers don't receive terminal markup.
func PlainAddress(addr Address) string {
	if addr.Address == "" {
		return ""
	}
	if addr.Decimal != 0 {
		return fmt.Sprintf("%s (%s - %d)", addr.Address, addr.Desc, addr.Decimal)
	}
	if addr.Desc != "" {
		return fmt.Sprintf("%s (%s)", addr.Address, addr.Desc)
	}
	return addr.Address
}

// VerboseAddress formats an Address for terminal display. The description is
// wrapped in ANSI color via NameWithColor. Do NOT use the output as data
// (e.g. JSON) — use PlainAddress for that.
func VerboseAddress(addr Address) string {
	if addr.Address == "" {
		return ""
	}
	if addr.Decimal != 0 {
		return fmt.Sprintf(
			"%s (%s)",
			addr.Address,
			NameWithColor(fmt.Sprintf("%s - %d", addr.Desc, addr.Decimal)),
		)
	}
	return fmt.Sprintf("%s (%s)", addr.Address, NameWithColor(addr.Desc))
}

// PlainValue returns a human-readable string for a single decoded ABI value
// with no ANSI color codes. Use in build/data phases.
func PlainValue(value Value) string {
	switch value.Kind {
	case DisplayAddress:
		return PlainAddress(*value.Address)
	case DisplayToken:
		if label, ok := MaxUintLabel(value.Raw); ok {
			if value.Token.Symbol != "" {
				return fmt.Sprintf("%s (%s, all %s)", value.Raw, label, value.Token.Symbol)
			}
			return fmt.Sprintf("%s (%s)", value.Raw, label)
		}
		human := BigToFloatString(StringToBig(value.Raw), value.Token.Decimal)
		if value.Token.Symbol != "" {
			return fmt.Sprintf("%s (%s %s)", value.Raw, human, value.Token.Symbol)
		}
		return fmt.Sprintf("%s (%s)", value.Raw, human)
	case DisplayInteger:
		return ReadableNumber(value.Raw)
	default:
		return value.Raw
	}
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
