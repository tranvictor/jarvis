package gateways

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	ethcommon "github.com/ethereum/go-ethereum/common"

	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	jarvisutil "github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/walletconnect"
)

// ErrMissingChainForSwitch is returned when a gateway's SwitchChain is
// called with a CAIP-2 chain that jarvis has no supported network
// entry for. Wrapped into ErrChainNotSupported so the session layer
// reports WC 5100.
func errChainUnknown(chain string) error {
	return fmt.Errorf("%w: %s is not a jarvis-known chain",
		walletconnect.ErrChainNotSupported, chain)
}

// networkForChain returns the jarvis Network for a CAIP-2 string, or
// a wrapped ErrChainNotSupported on miss.
func networkForChain(chain string) (jarvisnetworks.Network, error) {
	id, err := walletconnect.ParseChain(chain)
	if err != nil {
		return nil, err
	}
	net, err := jarvisnetworks.GetNetworkByID(id)
	if err != nil {
		return nil, errChainUnknown(chain)
	}
	return net, nil
}

// allKnownEip155Chains returns the CAIP-2 strings for every network
// jarvis has compiled-in support for. Used by every gateway's Chains()
// output unless the gateway narrows further.
func allKnownEip155Chains() []string {
	nets := jarvisnetworks.GetSupportedNetworks()
	out := make([]string, 0, len(nets))
	for _, n := range nets {
		out = append(out, walletconnect.ChainString(n.GetChainID()))
	}
	return out
}

// ensureSameAddress confirms tx.From (which the dApp sent) matches the
// address the session is supposed to act on behalf of. dApps are
// supposed to send the wallet's own address here, but some sloppy
// dApps omit it. We accept empty From (fill it in) but reject
// mismatches to avoid surprising the user.
func ensureSameAddress(txFrom, gatewayAddr string) error {
	if txFrom == "" {
		return nil
	}
	if !strings.EqualFold(txFrom, gatewayAddr) {
		return fmt.Errorf("tx.from %s does not match session account %s",
			txFrom, gatewayAddr)
	}
	return nil
}

// bigOrZero returns v, or a zero *big.Int if v is nil. Callers can
// then safely do arithmetic without a nil check.
func bigOrZero(v *big.Int) *big.Int {
	if v == nil {
		return new(big.Int)
	}
	return v
}

// hex0x returns hex-prefixed lower-case encoding of b, suitable for
// returning to a dApp as a signature / tx hash.
func hex0x(b []byte) string { return "0x" + hex.EncodeToString(b) }

// bytesToAddr converts an already-normalised lowercase hex string
// into a common.Address for go-ethereum APIs.
func bytesToAddr(hexStr string) ethcommon.Address {
	return ethcommon.HexToAddress(hexStr)
}

// shortLabel renders an address's address-book label (if any) in the
// form "0xabcd...1234 (Alice)" so confirm prompts don't just show
// opaque hex.
func shortLabel(addr string, network jarvisnetworks.Network) string {
	if addr == "" {
		return "(contract creation)"
	}
	ja := jarvisutil.GetJarvisAddress(addr, network)
	if ja.Desc != "" {
		return fmt.Sprintf("%s (%s)", addr, ja.Desc)
	}
	return addr
}
