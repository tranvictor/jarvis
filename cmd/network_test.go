package cmd

import (
	"strings"
	"testing"
)

func TestNodeHostStripsPathAndKey(t *testing.T) {
	cases := map[string]string{
		"https://ropsten.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5": "ropsten.infura.io",
		" https://rpc2-testnet.bitfi.xyz":                               "rpc2-testnet.bitfi.xyz",
		"http://127.0.0.1:8545":                                         "127.0.0.1:8545",
		"not-a-url":                                                     "not-a-url",
	}
	for in, want := range cases {
		got := nodeHost(in)
		if got != want {
			t.Errorf("nodeHost(%q) = %q, want %q", in, got, want)
		}
		if strings.Contains(got, "247128ae") {
			t.Errorf("nodeHost leaked the API key: %q", got)
		}
	}
}
