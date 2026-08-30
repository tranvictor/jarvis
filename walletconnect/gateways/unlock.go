package gateways

import (
	"fmt"
	"sync"

	"github.com/tranvictor/jarvis/accounts"
	jtypes "github.com/tranvictor/jarvis/accounts/types"
	"github.com/tranvictor/jarvis/util/account"
	"github.com/tranvictor/jarvis/walletconnect"
)

// unlockCache lazily unlocks a local wallet and reuses the Account.
// EOA sets locked so concurrent SwitchChain/sign can't race; Classic
// and Safe omit the mutex (session dispatch is synchronous).
type unlockCache struct {
	mu       sync.Mutex
	locked   bool
	unlocked *account.Account
	acc      jtypes.AccDesc
	ui       walletconnect.UI
	prompt   string
}

func (c *unlockCache) unlock() (*account.Account, error) {
	if c.locked {
		c.mu.Lock()
		defer c.mu.Unlock()
	}
	if c.unlocked != nil {
		return c.unlocked, nil
	}
	c.ui.Info("%s", c.prompt)
	ac, err := accounts.UnlockAccount(c.acc)
	if err != nil {
		return nil, fmt.Errorf("unlock wallet: %w", err)
	}
	c.unlocked = ac
	return ac, nil
}
