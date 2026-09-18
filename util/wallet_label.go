package util

import (
	"strings"
	"sync"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

// WalletNamer names addresses Jarvis controls (local wallets in
// ~/.jarvis/<address>.json). desc is the user-entered wallet description;
// kind is "ledger" / "trezor" / "keystore" / …. cmd/util registers
// accounts.Lookup; tests stub this.
type WalletNamer func(addr string) (desc, kind string, ok bool)

var (
	walletNamerMu sync.RWMutex
	walletNamer   WalletNamer
)

// SetWalletNamer installs the local-wallet lookup used by GetJarvisAddress.
func SetWalletNamer(fn WalletNamer) {
	walletNamerMu.Lock()
	defer walletNamerMu.Unlock()
	walletNamer = fn
}

func lookupWallet(addr string) (desc, kind string, ok bool) {
	walletNamerMu.RLock()
	fn := walletNamer
	walletNamerMu.RUnlock()
	if fn == nil {
		return "", "", false
	}
	return fn(addr)
}

const yourWalletTag = "your wallet"

// applyWalletLabel adds the wallet description and a "your wallet" marker
// so a signer Jarvis controls is named even when it is not in addresses.json.
func applyWalletLabel(a jarviscommon.Address, walletDesc, walletKind string) jarviscommon.Address {
	if jarviscommon.IsZeroAddress(a.Address) {
		return a
	}
	a.Private = true
	if strings.Contains(strings.ToLower(a.Desc), yourWalletTag) {
		return a
	}
	walletDesc = strings.TrimSpace(walletDesc)
	walletKind = strings.TrimSpace(walletKind)
	name := walletDesc
	if name == "" {
		name = walletKind
	}
	if jarviscommon.IsKnownAddress(a) {
		a.Desc = strings.TrimSpace(a.Desc) + " - " + yourWalletTag
		return a
	}
	if name != "" {
		a.Desc = name + " - " + yourWalletTag
		return a
	}
	a.Desc = yourWalletTag
	return a
}
