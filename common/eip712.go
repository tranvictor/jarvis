package common

import "github.com/ethereum/go-ethereum/crypto"

// EIP712Digest returns keccak256(0x19 || 0x01 || domainSeparator || structHash),
// the EIP-712 typed-data digest signed by wallets and Gnosis Safe owners.
func EIP712Digest(domainSeparator, structHash [32]byte) [32]byte {
	buf := make([]byte, 0, 2+32+32)
	buf = append(buf, 0x19, 0x01)
	buf = append(buf, domainSeparator[:]...)
	buf = append(buf, structHash[:]...)
	var out [32]byte
	copy(out[:], crypto.Keccak256(buf))
	return out
}
