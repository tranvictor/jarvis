package cmd

import (
	"fmt"
)

func indent(nospace int, strs []string) string {
	if len(strs) == 0 {
		return ""
	}

	if len(strs) == 1 {
		return strs[0]
	}

	indentation := ""
	for i := 0; i < nospace; i++ {
		indentation += " "
	}
	result := ""
	for i, str := range strs {
		result += fmt.Sprintf("\n%s%d. %s", indentation, i, str)
	}
	result += "\n"
	return result
}
