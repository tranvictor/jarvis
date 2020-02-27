package kyber

import (
	"fmt"

	"github.com/tranvictor/jarvis/util"
)

func getAddressFromParams(args []string, index int) (string, string, error) {
	if len(args) < 2 {
		return "", "", fmt.Errorf("only %d params are provided. %d params are needed.", len(args), index+1)
	}
	return util.GetAddressFromString(args[index])
}
