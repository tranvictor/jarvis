package util

import (
	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/util"
)

func init() {
	util.SetWalletNamer(AccountsWalletNamer)
}

// AccountsWalletNamer is the production WalletNamer: ~/.jarvis wallet records.
func AccountsWalletNamer(addr string) (desc, kind string, ok bool) {
	acc, ok := accounts.Lookup(addr)
	if !ok {
		return "", "", false
	}
	return acc.Desc, acc.Kind, true
}
