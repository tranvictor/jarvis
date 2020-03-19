package cmd

import (
	"fmt"
	"strings"
)

func indent(nospace int, str string) string {
	indentation := ""
	for i := 0; i < nospace; i++ {
		indentation += " "
	}
	return strings.ReplaceAll(str, "\n", fmt.Sprintf("\n%s", indentation))
}
