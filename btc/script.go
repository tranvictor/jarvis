package btc

import (
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

// DecodeAddress parses s and refuses addresses of another chain. btcutil
// already rejects wrong checksums and mixed-case bech32; the explicit
// IsForNet check stops a testnet address being accepted on mainnet.
func DecodeAddress(s string, params *chaincfg.Params) (btcutil.Address, error) {
	addr, err := btcutil.DecodeAddress(s, params)
	if err != nil {
		if other := otherNetwork(s, params); other != "" {
			return nil, fmt.Errorf("address %q belongs to %s, but this wallet is on %s (wrong network)", s, other, params.Name)
		}
		return nil, fmt.Errorf("invalid bitcoin address %q for %s: %w", s, params.Name, err)
	}
	if !addr.IsForNet(params) {
		return nil, fmt.Errorf("address %q belongs to another network than %s (wrong network)", s, params.Name)
	}
	return addr, nil
}

func otherNetwork(s string, not *chaincfg.Params) string {
	for _, p := range []*chaincfg.Params{&chaincfg.MainNetParams, &chaincfg.TestNet3Params, &chaincfg.SigNetParams, &chaincfg.RegressionNetParams} {
		if p.Name == not.Name {
			continue
		}
		if a, err := btcutil.DecodeAddress(s, p); err == nil && a.IsForNet(p) {
			return p.Name
		}
	}
	return ""
}

// PayTo returns the output script that pays addr.
func PayTo(addr btcutil.Address) ([]byte, error) {
	return txscript.PayToAddrScript(addr)
}

// Sizes in vbytes of a P2WPKH spend. The signature is counted at its
// largest (72 bytes including the sighash byte) so a fee never lands below
// the rate that was asked for.
const (
	p2wpkhInputWU   = 4*41 + 108
	txOverheadWU    = 4*10 + 2
	dustRelayPerKvB = 3000
)

func OutputVSize(script []byte) int {
	return 8 + varIntSize(len(script)) + len(script)
}

func varIntSize(n int) int {
	switch {
	case n < 0xfd:
		return 1
	case n <= 0xffff:
		return 3
	}
	return 5
}

// EstimateVSize returns the virtual size of a transaction that spends
// nInputs P2WPKH outputs into the given output scripts.
func EstimateVSize(nInputs int, outputScripts [][]byte) int {
	wu := txOverheadWU + nInputs*p2wpkhInputWU
	for _, s := range outputScripts {
		wu += 4 * OutputVSize(s)
	}
	return (wu + 3) / 4
}

// DustThreshold is Bitcoin Core's default dust limit for an output paying
// script: an output is dust when it costs more than a third of its value to
// spend at the dust relay rate. P2WPKH is 294 sat, P2PKH 546, P2TR 330.
func DustThreshold(script []byte) int64 {
	size := OutputVSize(script)
	if isWitnessProgram(script) {
		size += 32 + 4 + 1 + 107/4 + 4
	} else {
		size += 32 + 4 + 1 + 107 + 4
	}
	return int64(size) * dustRelayPerKvB / 1000
}

func isWitnessProgram(script []byte) bool {
	if len(script) < 4 || len(script) > 42 {
		return false
	}
	v := script[0]
	if v != 0 && (v < 0x51 || v > 0x60) {
		return false
	}
	return int(script[1])+2 == len(script)
}

// IsP2WPKH reports whether script is a native segwit v0 key-hash script.
func IsP2WPKH(script []byte) bool {
	return len(script) == 22 && script[0] == 0x00 && script[1] == 0x14
}
