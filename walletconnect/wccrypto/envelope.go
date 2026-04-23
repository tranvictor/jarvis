package wccrypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

// WalletConnect v2 envelope codec.
//
// Every message on the relay is wrapped in a binary envelope that is
// then base64-encoded for transport over JSON-RPC. Two envelope types
// are defined in the sign-client spec:
//
//	type 0: symKey only.  envelope := 0x00 || nonce(12) || AEAD(plaintext)
//	type 1: sender pub.   envelope := 0x01 || senderPub(32) || nonce(12) || AEAD(plaintext)
//
// Both use ChaCha20-Poly1305 as the AEAD. Type 1 is only used for the
// wallet's response to wc_sessionPropose (so the dApp can perform ECDH
// and derive the session symKey); everything else — including the
// request side of the same propose round-trip — uses type 0.
//
// AEAD AAD is always the empty string. Nonce is 12 random bytes.

const (
	envelopeTypeSym      byte = 0
	envelopeTypeSenderPub byte = 1
	nonceSize                 = 12
	pubKeySize                = 32
)

// Encrypt wraps plaintext in a type-0 (symKey-only) envelope and
// returns the base64 string suitable for irn_publish.
func Encrypt(symKey [32]byte, plaintext []byte) (string, error) {
	aead, err := chacha20poly1305.New(symKey[:])
	if err != nil {
		return "", fmt.Errorf("new chacha20poly1305: %w", err)
	}
	var nonce [nonceSize]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	ct := aead.Seal(nil, nonce[:], plaintext, nil)
	env := make([]byte, 0, 1+nonceSize+len(ct))
	env = append(env, envelopeTypeSym)
	env = append(env, nonce[:]...)
	env = append(env, ct...)
	return base64.StdEncoding.EncodeToString(env), nil
}

// EncryptWithSender wraps plaintext in a type-1 (sender-public-key)
// envelope. Used by the wallet when responding to wc_sessionPropose —
// the dApp uses senderPub together with its own private key to derive
// the session symKey via ECDH.
func EncryptWithSender(symKey [32]byte, senderPub [32]byte, plaintext []byte) (string, error) {
	aead, err := chacha20poly1305.New(symKey[:])
	if err != nil {
		return "", fmt.Errorf("new chacha20poly1305: %w", err)
	}
	var nonce [nonceSize]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	ct := aead.Seal(nil, nonce[:], plaintext, nil)
	env := make([]byte, 0, 1+pubKeySize+nonceSize+len(ct))
	env = append(env, envelopeTypeSenderPub)
	env = append(env, senderPub[:]...)
	env = append(env, nonce[:]...)
	env = append(env, ct...)
	return base64.StdEncoding.EncodeToString(env), nil
}

// DecryptResult is what Decrypt returns. SenderPub is the zero array
// for type-0 envelopes and the peer's x25519 public key for type-1.
type DecryptResult struct {
	Plaintext []byte
	Type      byte
	SenderPub [32]byte
}

// Decrypt opens a base64'd WC envelope with the given symKey. Both
// type-0 and type-1 envelopes are accepted; the caller inspects
// result.Type to know whether SenderPub is populated.
func Decrypt(symKey [32]byte, encoded string) (*DecryptResult, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// Some clients emit URL-safe base64 without padding; be
		// permissive for input but strict for output.
		raw, err = base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("base64 decode envelope: %w", err)
		}
	}
	if len(raw) < 1 {
		return nil, fmt.Errorf("envelope is empty")
	}
	aead, err := chacha20poly1305.New(symKey[:])
	if err != nil {
		return nil, fmt.Errorf("new chacha20poly1305: %w", err)
	}
	switch raw[0] {
	case envelopeTypeSym:
		if len(raw) < 1+nonceSize+chacha20poly1305.Overhead {
			return nil, fmt.Errorf("type-0 envelope too short: %d bytes", len(raw))
		}
		nonce := raw[1 : 1+nonceSize]
		ct := raw[1+nonceSize:]
		pt, err := aead.Open(nil, nonce, ct, nil)
		if err != nil {
			return nil, fmt.Errorf("aead open (type 0): %w", err)
		}
		return &DecryptResult{Plaintext: pt, Type: envelopeTypeSym}, nil
	case envelopeTypeSenderPub:
		if len(raw) < 1+pubKeySize+nonceSize+chacha20poly1305.Overhead {
			return nil, fmt.Errorf("type-1 envelope too short: %d bytes", len(raw))
		}
		var pub [32]byte
		copy(pub[:], raw[1:1+pubKeySize])
		nonce := raw[1+pubKeySize : 1+pubKeySize+nonceSize]
		ct := raw[1+pubKeySize+nonceSize:]
		pt, err := aead.Open(nil, nonce, ct, nil)
		if err != nil {
			return nil, fmt.Errorf("aead open (type 1): %w", err)
		}
		return &DecryptResult{Plaintext: pt, Type: envelopeTypeSenderPub, SenderPub: pub}, nil
	default:
		return nil, fmt.Errorf("unknown envelope type %d", raw[0])
	}
}
