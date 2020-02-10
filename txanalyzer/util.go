package txanalyzer

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
)

func SplitEventArguments(args abi.Arguments) (abi.Arguments, abi.Arguments) {
	indexedArgs := abi.Arguments{}
	nonindexedArgs := abi.Arguments{}
	for _, arg := range args {
		if arg.Indexed {
			indexedArgs = append(indexedArgs, arg)
		} else {
			nonindexedArgs = append(nonindexedArgs, arg)
		}
	}
	return indexedArgs, nonindexedArgs
}
