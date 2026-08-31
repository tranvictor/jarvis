package common

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

func GetERC20ABI() *abi.ABI {
	result, _ := abi.JSON(strings.NewReader(erc20abi))
	return &result
}

func GetMultiSendABI() *abi.ABI {
	result, _ := abi.JSON(strings.NewReader(multisendabi))
	return &result
}

func GetEIP1967BeaconABI() *abi.ABI {
	result, _ := abi.JSON(strings.NewReader(eip1967beacon))
	return &result
}

func PackERC20Data(function string, params ...interface{}) ([]byte, error) {
	return GetERC20ABI().Pack(function, params...)
}

func HexToAddress(hex string) common.Address {
	return common.HexToAddress(hex)
}

func HexToAddresses(hexes []string) []common.Address {
	result := []common.Address{}
	for _, h := range hexes {
		result = append(result, common.HexToAddress(h))
	}
	return result
}

func HexToHash(hex string) common.Hash {
	return common.HexToHash(hex)
}

// LooksLikeAddress reports whether s is shaped like a raw on-chain address,
// as opposed to a human label/name that should be fuzzy-searched in the
// address book.
//
// This is the single decision point callers should use to choose between
// "resolve this exact address" and "search labels for this text". Treating
// a raw address as free text is what let fuzzy/full-text search silently
// swap in an unrelated address whose label happened to mention the
// requested one (or match a near-miss like the zero address) — see the
// address book bug where looking up a multisig address returned a
// different multisig because its description text referenced the first
// one, and where a "0x000...0003" input matched address(0) purely on
// string similarity.
//
// Jarvis is EVM-only today, so this simply checks for a 40 hex-character
// string (with or without a 0x prefix). Once non-EVM networks are
// supported, extend this (or dispatch on the target network) instead of
// scattering common.IsHexAddress calls across call sites.
func LooksLikeAddress(s string) bool {
	return common.IsHexAddress(strings.TrimSpace(s))
}
