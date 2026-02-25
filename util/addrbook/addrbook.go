// Package addrbook provides the AddressResolver interface and a set of
// ready-made implementations for mapping raw Ethereum hex addresses to
// human-readable names.
//
// Production code uses [Default], which queries the local bleve and DB
// address databases and enriches results with cached ERC20 metadata.
// Tests inject [Map] — a plain map that resolves to deterministic names
// without any network or database access.
//
// Future implementations (ENS, Etherscan labels, public address lists, …)
// should live here and implement the same interface so callers never need
// to change.
package addrbook

import (
	jarviscommon "github.com/tranvictor/jarvis/common"
)

// AddressResolver maps a raw Ethereum hex address to a human-readable
// jarviscommon.Address (hex + optional name/description + optional ERC20
// decimal). Abstracting this behind an interface lets any component that
// enriches addresses be tested deterministically without touching the local
// address databases or the network.
//
// Contract: if the address is not known, Desc must be set to "unknown".
type AddressResolver interface {
	Resolve(addr string) jarviscommon.Address
}
