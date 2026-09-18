package db

import (
	"fmt"
	"strings"
	"sync"
)

var (
	onceSource      sync.Once
	source          FuzzySource
	sourceByAddress map[string]AddressDesc
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
		source = make(FuzzySource, 0, len(addrs))
		sourceByAddress = make(map[string]AddressDesc, len(addrs))
		for addr, desc := range addrs {
			ad := AddressDesc{
				Address:      addr,
				Desc:         desc,
				SearchString: fmt.Sprintf("%s_%s", strings.Replace(desc, " ", "_", -1), addr),
			}
			source = append(source, ad)
			sourceByAddress[strings.ToLower(addr)] = ad
		}
	})
	return source
}

// lookupExact returns the AddressDesc registered for addr (case-insensitive
// exact match), bypassing fuzzy matching entirely.
//
// This exists so that an exact address input can never be "corrected" to a
// different, unrelated address whose label happens to contain a similar
// string (e.g. a multisig's description mentioning another address) or
// whose hex happens to be a near-miss (e.g. the zero address).
func lookupExact(addr string) (AddressDesc, bool) {
	NewFuzzySource()
	ad, ok := sourceByAddress[strings.ToLower(addr)]
	return ad, ok
}
