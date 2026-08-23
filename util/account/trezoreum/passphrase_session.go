package trezoreum

import (
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

const passphraseMask = "********"

type passphraseEntry struct {
	secret   string
	onDevice bool
}

// processPassphrase is the in-memory passphrase session for this Jarvis
// process. Nothing here is written to AccDesc or disk.
var processPassphrase = struct {
	mu        sync.Mutex
	sessionID []byte
	pending   *passphraseEntry
	byAddress map[string]passphraseEntry
}{
	byAddress: make(map[string]passphraseEntry),
}

func addrKey(addr common.Address) string {
	return addr.Hex()
}

func currentSessionID() []byte {
	processPassphrase.mu.Lock()
	defer processPassphrase.mu.Unlock()
	if len(processPassphrase.sessionID) == 0 {
		return nil
	}
	out := make([]byte, len(processPassphrase.sessionID))
	copy(out, processPassphrase.sessionID)
	return out
}

func storeSessionID(id []byte) {
	processPassphrase.mu.Lock()
	defer processPassphrase.mu.Unlock()
	if len(id) == 0 {
		processPassphrase.sessionID = nil
		return
	}
	processPassphrase.sessionID = append([]byte(nil), id...)
}

func forgetDeviceSession() {
	storeSessionID(nil)
}

func rememberPending(secret string, onDevice bool) {
	processPassphrase.mu.Lock()
	defer processPassphrase.mu.Unlock()
	processPassphrase.pending = &passphraseEntry{secret: secret, onDevice: onDevice}
}

func bindPassphrase(addr common.Address) {
	if addr == (common.Address{}) {
		return
	}
	processPassphrase.mu.Lock()
	defer processPassphrase.mu.Unlock()
	if processPassphrase.pending == nil {
		return
	}
	processPassphrase.byAddress[addrKey(addr)] = *processPassphrase.pending
}

func lookupCachedPassphrase(expected common.Address) (passphraseEntry, bool) {
	processPassphrase.mu.Lock()
	defer processPassphrase.mu.Unlock()
	if expected != (common.Address{}) {
		e, ok := processPassphrase.byAddress[addrKey(expected)]
		return e, ok
	}
	if processPassphrase.pending != nil {
		return *processPassphrase.pending, true
	}
	return passphraseEntry{}, false
}

// ResetPassphraseSession drops the device session id and any unbound
// pending secret, and forgets the cached secret for addr (if set) so
// the next unlock can ask for a different passphrase. Other addresses
// stay cached.
func ResetPassphraseSession(addr common.Address) {
	processPassphrase.mu.Lock()
	defer processPassphrase.mu.Unlock()
	processPassphrase.sessionID = nil
	processPassphrase.pending = nil
	if addr != (common.Address{}) {
		delete(processPassphrase.byAddress, addrKey(addr))
	}
}

func resetPassphraseSessionForTest() {
	processPassphrase.mu.Lock()
	defer processPassphrase.mu.Unlock()
	processPassphrase.sessionID = nil
	processPassphrase.pending = nil
	processPassphrase.byAddress = make(map[string]passphraseEntry)
}
