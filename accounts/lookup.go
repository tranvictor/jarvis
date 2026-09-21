package accounts

import (
	"errors"
	"regexp"
	"strings"
	"sync"

	ethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/accounts/types"
)

var hexAddrInPath = regexp.MustCompile(`(?i)(0x)?[0-9a-f]{40}`)

var errInvalidWalletPath = errors.New("invalid filename")

var (
	walletMu     sync.Mutex
	walletCache  map[string]types.AccDesc
	walletCached bool
	testWallets  map[string]types.AccDesc
)

func addressFromPath(path string) (string, error) {
	found := hexAddrInPath.FindAllString(path, -1)
	if found == nil {
		return "", errInvalidWalletPath
	}
	return found[0], nil
}

func canonicalWalletAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if !ethcommon.IsHexAddress(addr) {
		extracted, err := addressFromPath(addr)
		if err != nil {
			return ""
		}
		addr = extracted
		if !strings.HasPrefix(strings.ToLower(addr), "0x") {
			addr = "0x" + addr
		}
		if !ethcommon.IsHexAddress(addr) {
			return ""
		}
	}
	return strings.ToLower(ethcommon.HexToAddress(addr).Hex())
}

func indexWallets(src map[string]types.AccDesc) map[string]types.AccDesc {
	out := make(map[string]types.AccDesc, len(src)*2)
	add := func(addr string, acc types.AccDesc) {
		key := canonicalWalletAddr(addr)
		if key == "" || strings.TrimSpace(acc.Kind) == "" {
			return
		}
		out[key] = acc
	}
	for k, acc := range src {
		add(k, acc)
		add(acc.Address, acc)
	}
	return out
}

func invalidateWalletCache() {
	walletMu.Lock()
	walletCached = false
	walletCache = nil
	walletMu.Unlock()
}

// SetWalletsForTest replaces the on-disk wallet list. Pass nil to restore
// reading ~/.jarvis. Tests should t.Cleanup(func() { SetWalletsForTest(nil) }).
func SetWalletsForTest(wallets map[string]types.AccDesc) {
	walletMu.Lock()
	defer walletMu.Unlock()
	if wallets == nil {
		testWallets = nil
		return
	}
	testWallets = indexWallets(wallets)
}

// Lookup returns the local jarvis wallet for addr (a ~/.jarvis/<address>.json
// record), if Jarvis controls it. Address-book names are a separate database.
func Lookup(addr string) (types.AccDesc, bool) {
	key := canonicalWalletAddr(addr)
	if key == "" {
		return types.AccDesc{}, false
	}
	walletMu.Lock()
	defer walletMu.Unlock()
	if testWallets != nil {
		acc, ok := testWallets[key]
		return acc, ok
	}
	if !walletCached {
		walletCache = indexWallets(GetAccounts())
		walletCached = true
	}
	acc, ok := walletCache[key]
	return acc, ok
}
