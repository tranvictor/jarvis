package accounts

import (
	"fmt"
	"github.com/tranvictor/jarvis/accounts/types"
	"strings"
)

type FuzzySource []types.AccDesc

func (self FuzzySource) Len() int {
	return len(self)
}

func (self FuzzySource) String(i int) string {
	return fmt.Sprintf("%s_%s", self[i].Address, strings.Replace(self[i].Desc, " ", "_", -1))
}

func NewFuzzySource() FuzzySource {
	accounts := GetAccounts()
	result := FuzzySource{}
	for _, acc := range accounts {
		result = append(result, acc)
	}
	return result
}
