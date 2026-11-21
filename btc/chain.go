// Package btc holds the pieces of Jarvis that deal with native Bitcoin:
// chains, amounts, scripts and fee maths. It deliberately shares no types
// with the EVM side of Jarvis (networks.Network, accounts.AccDesc, ...).
package btc

import (
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
)

type Chain string

const (
	Mainnet Chain = "mainnet"
	Testnet Chain = "testnet"
	Signet  Chain = "signet"
	Regtest Chain = "regtest"
)

func ParseChain(s string) (Chain, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "mainnet", "main", "bitcoin":
		return Mainnet, nil
	case "testnet", "testnet3":
		return Testnet, nil
	case "signet":
		return Signet, nil
	case "regtest":
		return Regtest, nil
	}
	return "", fmt.Errorf("unknown bitcoin chain %q (use mainnet, testnet, signet or regtest)", s)
}

func (c Chain) Params() *chaincfg.Params {
	switch c {
	case Testnet:
		return &chaincfg.TestNet3Params
	case Signet:
		return &chaincfg.SigNetParams
	case Regtest:
		return &chaincfg.RegressionNetParams
	}
	return &chaincfg.MainNetParams
}

// CoinType is the BIP-44 coin type of the chain: 0 on mainnet, 1 on every
// test chain.
func (c Chain) CoinType() uint32 {
	if c == Mainnet {
		return 0
	}
	return 1
}

// DefaultEsplora is the public Esplora-compatible API used when the caller
// does not point Jarvis at their own. Regtest has no public instance.
func (c Chain) DefaultEsplora() string {
	switch c {
	case Mainnet:
		return "https://mempool.space/api"
	case Testnet:
		return "https://mempool.space/testnet/api"
	case Signet:
		return "https://mempool.space/signet/api"
	}
	return ""
}

const hardened = 0x80000000

// AccountPath is the BIP-84 account path m/84'/coin'/account'.
func (c Chain) AccountPath(account uint32) []uint32 {
	return []uint32{84 + hardened, c.CoinType() + hardened, account + hardened}
}

func FormatPath(path []uint32) string {
	var b strings.Builder
	b.WriteString("m")
	for _, p := range path {
		if p >= hardened {
			fmt.Fprintf(&b, "/%d'", p-hardened)
		} else {
			fmt.Fprintf(&b, "/%d", p)
		}
	}
	return b.String()
}

func ParsePath(s string) ([]uint32, error) {
	s = strings.TrimSpace(s)
	if s == "m" || s == "" {
		return nil, nil
	}
	parts := strings.Split(strings.TrimPrefix(s, "m/"), "/")
	out := make([]uint32, 0, len(parts))
	for _, p := range parts {
		h := false
		if strings.HasSuffix(p, "'") || strings.HasSuffix(p, "h") {
			h = true
			p = p[:len(p)-1]
		}
		var n uint32
		if _, err := fmt.Sscanf(p, "%d", &n); err != nil || n >= hardened {
			return nil, fmt.Errorf("bad derivation path element %q in %q", p, s)
		}
		if h {
			n += hardened
		}
		out = append(out, n)
	}
	return out, nil
}
