package util

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	. "github.com/tranvictor/jarvis/networks"
)

type CMD int

const (
	BalanceOf CMD = iota
	TokenBalance
)

type GrammarTree struct {
	Cmd    CMD
	Params []GrammarTree
}

func (gt *GrammarTree) Execute() (result string, t abi.Type, err error) {
	return "", abi.Type{}, fmt.Errorf("not implemented")
}

func ParseGrammarTree(input string) (*GrammarTree, error) {
	return nil, fmt.Errorf("not implemented")
}

func IsInlineScript(input string) bool {
	// TODO: implement it
	return false
}

func InterpretInput(input string, network Network) (string, error) {
	// checking if input is an inline script
	if IsInlineScript(input) {
		gramTree, err := ParseGrammarTree(input)
		if err != nil {
			return "", err
		}
		result, t, err := gramTree.Execute()
		if err != nil {
			return "", err
		}

		resultInputStr, err := ConvertEthereumTypeToInputString(t, result)
		if err != nil {
			return "", err
		}

		return resultInputStr, nil
	} else {
		// else return as is
		return input, nil
	}
}
