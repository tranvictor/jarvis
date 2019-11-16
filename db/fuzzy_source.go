package db

import (
	"fmt"
	"strings"
)

type AddressDesc struct {
	Address string
	Desc    string
}

type FuzzySource []AddressDesc

func (self FuzzySource) Len() int {
	return len(self)
}

func (self FuzzySource) String(i int) string {
	return fmt.Sprintf("%s_%s", strings.Replace(self[i].Desc, " ", "_", -1), self[i].Address)
}

func NewFuzzySource() FuzzySource {
	addrs := AllAddresses()
	result := FuzzySource{}
	for addr, desc := range addrs {
		result = append(result, AddressDesc{
			Address: addr,
			Desc:    desc,
		})
	}
	return result
}

func NewTokenFuzzySource() FuzzySource {
	addrs := AllTokenAddresses()
	result := FuzzySource{}
	for addr, desc := range addrs {
		result = append(result, AddressDesc{
			Address: addr,
			Desc:    desc,
		})
	}
	return result
}
