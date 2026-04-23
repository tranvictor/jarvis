// Package wccrypto groups the WalletConnect v2 cryptographic
// primitives: x25519 ECDH for the session handshake, HKDF-SHA256 for
// symmetric key derivation, ChaCha20-Poly1305 for envelope
// encryption, and the relay-auth JWT (ed25519 + did:key).
//
// The package is self-contained and has no jarvis dependencies so it
// can be unit-tested in isolation. Everything lives off golang.org/x
// /crypto, which is already an indirect jarvis dep via go-ethereum.
package wccrypto
