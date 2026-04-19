package safe

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
)

// IsGnosisSafe returns true when a is the ABI of a Gnosis Safe / Safe
// (v1.1.x .. v1.4.x). The detection uses the smallest stable subset of
// methods that distinguishes Safe from Gnosis Classic (which uses
// `submitTransaction`/`confirmTransaction`/`required`) and from regular
// contracts.
func IsGnosisSafe(a *abi.ABI) bool {
	if a == nil {
		return false
	}
	required := []string{
		"execTransaction",
		"approveHash",
		"getOwners",
		"getThreshold",
		"nonce",
		"domainSeparator",
	}
	for _, m := range required {
		if _, found := a.Methods[m]; !found {
			return false
		}
	}
	return true
}
