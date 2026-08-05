package db

import (
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

// getAddressMatches resolves input against source.
//
// If input looks like a raw address (see jarviscommon.LooksLikeAddress), it
// is resolved by exact key lookup only, via exact. Address-shaped input
// must never fall through to fuzzy matching: fuzzy scoring on other
// entries' labels (which may themselves mention unrelated addresses, or
// simply look similar, e.g. the zero address) can otherwise return a
// completely different address than the one requested.
//
// Free-text input (names, partial labels, ...) is fuzzy-matched as before.
func getAddressMatches(input string, source FuzzySource, exact func(string) (AddressDesc, bool)) ([]AddressDesc, []int) {
	if jarviscommon.LooksLikeAddress(input) {
		if ad, ok := exact(input); ok {
			return []AddressDesc{ad}, []int{1}
		}
		return []AddressDesc{}, []int{}
	}

	matches := fuzzy.FindFrom(strings.Replace(input, " ", "_", -1), source)
	result := []AddressDesc{}
	scores := []int{}
	for i := 0; i < 10; i++ {
		if i < len(matches) {
			result = append(result, source[matches[i].Index])
			scores = append(scores, matches[i].Score)
		} else {
			break
		}
	}
	return result, scores
}

func GetAddresses(input string) ([]AddressDesc, []int) {
	source := NewFuzzySource()
	return getAddressMatches(input, source, lookupExact)
}

func GetAddress(input string) (AddressDesc, error) {
	matches, _ := GetAddresses(input)
	if len(matches) == 0 {
		return AddressDesc{}, fmt.Errorf("No address is found with '%s'", input)
	}
	return matches[0], nil
}

func GetTokenAddress(input string) (AddressDesc, error) {
	source := NewTokenFuzzySource()
	matches, _ := getAddressMatches(input, source, lookupTokenExact)
	if len(matches) == 0 {
		return AddressDesc{}, fmt.Errorf("No address is found with '%s'", input)
	}
	return matches[0], nil
}

func AllTokenAddresses() map[string]string {
	result := map[string]string{}
	for addr, desc := range TOKENS {
		result[strings.ToLower(addr)] = desc
	}
	return result
}

func AllAddresses() map[string]string {
	addrs := NewDefaultAddressDatabase().Data
	tokenAddrs := AllTokenAddresses()
	result := map[string]string{}
	for addr, desc := range tokenAddrs {
		result[strings.ToLower(addr)] = desc
	}
	for addr, desc := range addrs {
		result[strings.ToLower(addr.Hex())] = desc
	}
	return result
}
