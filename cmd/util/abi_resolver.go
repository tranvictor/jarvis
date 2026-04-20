package util

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/tranvictor/jarvis/accounts"
	jtypes "github.com/tranvictor/jarvis/accounts/types"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/ens"
)

// ABIResolver abstracts address-lookup and ABI-fetching operations that otherwise
// require live network or database access. Command Run functions depend on this
// interface so that tests can substitute a stub and exercise logic offline.
type ABIResolver interface {
	GetAddressFromString(str string) (addr, name string, err error)
	GetMatchingAddress(str string) (addr, name string, err error)
	GetABI(addr string, network jarvisnetworks.Network) (*abi.ABI, error)
	ConfigToABI(addr string, forceERC20 bool, customABI string, network jarvisnetworks.Network) (*abi.ABI, error)
}

// DefaultABIResolver delegates every method to the util package's production
// implementations. It is injected by the preprocessing hooks so that all
// production invocations transparently hit the real databases and Etherscan.
type DefaultABIResolver struct{}

func (DefaultABIResolver) GetAddressFromString(str string) (string, string, error) {
	return util.GetAddressFromString(str)
}

func (DefaultABIResolver) GetMatchingAddress(str string) (string, string, error) {
	return util.GetMatchingAddress(str)
}

func (DefaultABIResolver) GetABI(addr string, network jarvisnetworks.Network) (*abi.ABI, error) {
	return util.GetABI(addr, network)
}

func (DefaultABIResolver) ConfigToABI(addr string, forceERC20 bool, customABI string, network jarvisnetworks.Network) (*abi.ABI, error) {
	return util.ConfigToABI(addr, forceERC20, customABI, network)
}

// ResolveAccount looks up a local wallet by the given keyword.
//
// When the keyword looks like a .eth name (per ens.IsLikelyENSName),
// ENS is queried first and the resulting hex address is used for the
// wallet lookup — fuzzy-matching a .eth string against local wallet
// descriptions never yields a useful hit, and trying it first only
// produces misleading errors like "Couldn't get ABI of <ens-resolved
// address>" when the fallback multisig path kicks in.
//
// For every other keyword shape (hex address, address-book label, free
// text) we use the existing fuzzy lookup directly: accounts.GetAccount
// already handles both address and description matching and is what
// jarvis has done historically.
//
// On success the resolved hex address is returned alongside the account
// so callers can pin config.From / downstream keywords to a canonical
// form for subsequent resolution or logging.
func ResolveAccount(resolver ABIResolver, keyword string) (acc jtypes.AccDesc, resolved string, err error) {
	if ens.IsLikelyENSName(keyword) {
		if resolver != nil {
			if addr, _, rerr := resolver.GetAddressFromString(keyword); rerr == nil {
				if a, aerr := accounts.GetAccount(addr); aerr == nil {
					return a, addr, nil
				}
			}
		}
		// ENS lookup didn't land on a local wallet. Don't fall through
		// to fuzzy-matching the raw ".eth" string: it would at best
		// return a coincidental hit. Report the original "no account"
		// error so the caller can decide whether to try the multisig
		// path next.
		return jtypes.AccDesc{}, "", fmt.Errorf("no local wallet for ENS name %q", keyword)
	}
	acc, err = accounts.GetAccount(keyword)
	if err != nil {
		return jtypes.AccDesc{}, "", err
	}
	return acc, acc.Address, nil
}
