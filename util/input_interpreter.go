package util

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"

	jarvisnetworks "github.com/tranvictor/jarvis/networks"
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

func InterpretInput(input string, network jarvisnetworks.Network) (string, error) {
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

func SplitArrayOrTupleStringInput(input string) ([]string, error) {
	input = strings.TrimSpace(input)
	if len(input) < 2 {
		return nil, errors.New("invalid input, too short to be array/tuple")
	}

	// Check first and last characters
	first, last := input[0], input[len(input)-1]

	// Confirm it's either parentheses or square brackets
	validEnclosing :=
		(first == '(' && last == ')') ||
			(first == '[' && last == ']')
	if !validEnclosing {
		return nil, fmt.Errorf("input must start with '(' or '[' and end with corresponding ')' or ']'")
	}

	// Strip the outermost brackets/parentheses
	content := strings.TrimSpace(input[1 : len(input)-1])
	if len(content) == 0 {
		// Empty array/tuple
		return []string{}, nil
	}

	var result []string
	var current strings.Builder

	bracketLevel := 0 // nesting level for square brackets [ ]
	parenLevel := 0   // nesting level for parentheses ( )

	for i, r := range content {
		switch r {
		case '[':
			bracketLevel++
			current.WriteRune(r)
		case ']':
			bracketLevel--
			if bracketLevel < 0 {
				return nil, fmt.Errorf("mismatched ']' at position %d", i)
			}
			current.WriteRune(r)
		case '(':
			parenLevel++
			current.WriteRune(r)
		case ')':
			parenLevel--
			if parenLevel < 0 {
				return nil, fmt.Errorf("mismatched ')' at position %d", i)
			}
			current.WriteRune(r)
		case ',':
			// Only split on comma if top-level
			if bracketLevel == 0 && parenLevel == 0 {
				// finish current token, add to result
				trimmed := strings.TrimSpace(current.String())
				if trimmed != "" {
					result = append(result, trimmed)
				}
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
	}

	// Add the last element
	if current.Len() > 0 {
		trimmed := strings.TrimSpace(current.String())
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	// If we haven't closed all brackets or parentheses, it's malformed
	if bracketLevel != 0 || parenLevel != 0 {
		return nil, errors.New("unbalanced brackets or parentheses")
	}

	return result, nil
}
