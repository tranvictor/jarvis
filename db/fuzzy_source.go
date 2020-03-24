package db

import (
	"fmt"
	"strings"
	"sync"
)

var (
	onceSource      sync.Once
	source          FuzzySource
	onceTokenSource sync.Once
	tokenSource     FuzzySource
)

type AddressDesc struct {
	Address      string
	Desc         string
	SearchString string
}

type FuzzySource []AddressDesc

func (self FuzzySource) Len() int {
	return len(self)
}

func (self FuzzySource) String(i int) string {
	return self[i].SearchString
}

func NewFuzzySource() FuzzySource {
	onceSource.Do(func() {
		addrs := AllAddresses()
		source = FuzzySource{}
		for addr, desc := range addrs {
			source = append(source, AddressDesc{
				Address:      addr,
				Desc:         desc,
				SearchString: fmt.Sprintf("%s_%s", strings.Replace(desc, " ", "_", -1), addr),
			})
		}
	})
	return source
}

func NewTokenFuzzySource() FuzzySource {
	onceTokenSource.Do(func() {
		addrs := AllTokenAddresses()
		tokenSource = FuzzySource{}
		for addr, desc := range addrs {
			tokenSource = append(tokenSource, AddressDesc{
				Address:      addr,
				Desc:         desc,
				SearchString: fmt.Sprintf("%s_%s", strings.Replace(desc, " ", "_", -1), addr),
			})
		}
	})
	return tokenSource
}
