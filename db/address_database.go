package db

import (
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"
	"github.com/tranvictor/jarvis/txanalyzer"
)

func getAddressMatches(input string, source FuzzySource) ([]AddressDesc, []int) {
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
	return getAddressMatches(input, source)
}

func GetAddress(input string) (AddressDesc, error) {
	source := NewFuzzySource()
	matches, _ := getAddressMatches(input, source)
	if len(matches) == 0 {
		return AddressDesc{}, fmt.Errorf("No address is found with '%s'", input)
	}
	return matches[0], nil
}

func GetTokenAddress(input string) (AddressDesc, error) {
	source := NewTokenFuzzySource()
	matches, _ := getAddressMatches(input, source)
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
	addrs := txanalyzer.NewDefaultAddressDatabase().Data
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
