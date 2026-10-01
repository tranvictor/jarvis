package accounts

import (
	"strings"
	"sync"
	"testing"

	"github.com/tranvictor/jarvis/accounts/types"
	"github.com/tranvictor/jarvis/util/account"
)

const (
	unlockAddrA = "0xa3759774994F5012E5d725dCC1B96750945C793f"
	unlockAddrB = "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"
	unlockPath0 = "m/44'/60'/0'/0/0"
	unlockPath1 = "m/44'/60'/0'/0/1"
)

// A second unlock of the same hardware wallet must reuse the first signer:
// a fresh one would try to claim a USB interface the first still holds.
func TestUnlockAccountReusesHardwareWallet(t *testing.T) {
	for _, kind := range []string{"trezor", "ledger", "ledger-live"} {
		t.Run(kind, func(t *testing.T) {
			t.Cleanup(ForgetUnlockedAccounts)
			ad := types.AccDesc{Address: unlockAddrA, Kind: kind, Derpath: unlockPath0}
			first, err := UnlockAccount(ad)
			if err != nil {
				t.Fatal(err)
			}
			ad.Address = strings.ToLower(ad.Address)
			ad.Desc = "renamed"
			second, err := UnlockAccount(ad)
			if err != nil {
				t.Fatal(err)
			}
			if first != second {
				t.Fatal("same device/path/address unlocked twice must return the same account")
			}
		})
	}
}

func TestUnlockAccountKeepsDistinctWalletsApart(t *testing.T) {
	t.Cleanup(ForgetUnlockedAccounts)
	base := types.AccDesc{Address: unlockAddrA, Kind: "trezor", Derpath: unlockPath0}
	first, err := UnlockAccount(base)
	if err != nil {
		t.Fatal(err)
	}
	for name, ad := range map[string]types.AccDesc{
		"other address": {Address: unlockAddrB, Kind: "trezor", Derpath: unlockPath0},
		"other path":    {Address: unlockAddrA, Kind: "trezor", Derpath: unlockPath1},
		"other device":  {Address: unlockAddrA, Kind: "ledger", Derpath: unlockPath0},
	} {
		got, err := UnlockAccount(ad)
		if err != nil {
			t.Fatalf("%s: %s", name, err)
		}
		if got == first {
			t.Fatalf("%s must not reuse the %s account", name, base.Kind)
		}
		if !strings.EqualFold(got.Address().Hex(), ad.Address) {
			t.Fatalf("%s: account address %s, want %s", name, got.Address().Hex(), ad.Address)
		}
	}
}

func TestUnlockAccountDoesNotCacheFailures(t *testing.T) {
	t.Cleanup(ForgetUnlockedAccounts)
	bad := types.AccDesc{Address: unlockAddrA, Kind: "trezor", Derpath: "not a path"}
	if _, err := UnlockAccount(bad); err == nil {
		t.Fatal("invalid derivation path must fail")
	}
	if _, err := UnlockAccount(bad); err == nil {
		t.Fatal("a failure must not be cached as a success")
	}
	if n := unlockedCount(); n != 0 {
		t.Fatalf("failed unlocks left %d cached accounts", n)
	}
}

func TestUnlockAccountConcurrentCallsShareOneSigner(t *testing.T) {
	t.Cleanup(ForgetUnlockedAccounts)
	ad := types.AccDesc{Address: unlockAddrA, Kind: "trezor", Derpath: unlockPath0}
	const n = 16
	got := make([]*account.Account, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i], _ = UnlockAccount(ad)
		}(i)
	}
	wg.Wait()
	for i := 1; i < n; i++ {
		if got[i] == nil || got[i] != got[0] {
			t.Fatalf("concurrent unlock %d returned a different account", i)
		}
	}
}

func TestUnlockAccountDoesNotCacheKeystores(t *testing.T) {
	t.Cleanup(ForgetUnlockedAccounts)
	if _, ok := unlockCacheKey(types.AccDesc{Address: unlockAddrA, Kind: "keystore", Keypath: "/tmp/k"}); ok {
		t.Fatal("keystore accounts must prompt for the password on every unlock")
	}
}
