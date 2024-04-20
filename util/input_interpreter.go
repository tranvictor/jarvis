package util

import (
	"fmt"
	"strings"

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

func SplitInputParamStr(input string) (result []string, err error) {
	var currentToken strings.Builder
	inParenthesesCount := 0

	for _, char := range input {
		switch char {
		case '(':
			inParenthesesCount += 1
			currentToken.WriteRune(char)
		case ')':
			if inParenthesesCount == 0 {
				return nil, fmt.Errorf("invalid input, your input has more ) than prior (")
			}
			inParenthesesCount -= 1
			currentToken.WriteRune(char)
		case ',':
			if inParenthesesCount > 0 {
				// If inside parentheses, treat comma as a normal character
				currentToken.WriteRune(char)
			} else {
				// Otherwise, it's a delimiter
				result = append(result, currentToken.String())
				currentToken.Reset()
			}
		default:
			currentToken.WriteRune(char)
		}
	}

	// Append the last token
	if currentToken.Len() > 0 {
		result = append(result, currentToken.String())
	}

	return result, nil
}
