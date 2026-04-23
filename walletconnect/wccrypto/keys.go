package wccrypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
	"io"
)

// KeyPair is an x25519 keypair as used by WC v2 for its session
// proposal/settle ECDH handshake. Private key material never leaves
// the process and is regenerated on every pairing — there is no
// long-term identity key.
type KeyPair struct {
	Public  [32]byte
	Private [32]byte
}

// NewKeyPair generates a fresh x25519 keypair from crypto/rand.
func NewKeyPair() (*KeyPair, error) {
	var priv [32]byte
	if _, err := io.ReadFull(rand.Reader, priv[:]); err != nil {
		return nil, fmt.Errorf("read random bytes: %w", err)
	}
	// RFC 7748 §5: clamp the scalar.
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("derive public key: %w", err)
	}
	kp := &KeyPair{}
	copy(kp.Private[:], priv[:])
	copy(kp.Public[:], pub)
	return kp, nil
}

// DeriveSessionSymKey derives the 32-byte symmetric key used to encrypt
// session-topic envelopes after the propose response, as defined by
// the WC v2 sign-client spec:
//
//	shared    = X25519(ourPriv, peerPub)
//	symKey    = HKDF-SHA256(ikm=shared, salt=nil, info=nil, size=32)
//	sessionTopic = sha256(symKey)
//
// HKDF is used in "extract-and-expand" mode with an empty salt, matching
// the behaviour of @walletconnect/utils' deriveSymKey helper.
func DeriveSessionSymKey(ourPriv [32]byte, peerPub [32]byte) (symKey [32]byte, topic string, err error) {
	shared, err := curve25519.X25519(ourPriv[:], peerPub[:])
	if err != nil {
		return symKey, "", fmt.Errorf("x25519 shared secret: %w", err)
	}
	rd := hkdf.New(sha256.New, shared, nil, nil)
	if _, err := io.ReadFull(rd, symKey[:]); err != nil {
		return symKey, "", fmt.Errorf("hkdf expand: %w", err)
	}
	h := sha256.Sum256(symKey[:])
	return symKey, hex.EncodeToString(h[:]), nil
}

// TopicForSymKey computes the WC v2 topic associated with a symmetric
// key, i.e. sha256(symKey) as a hex string. Used for both pairing
// topics (whose symKey came from the wc: URI) and session topics
// (whose symKey came from DeriveSessionSymKey).
func TopicForSymKey(symKey [32]byte) string {
	h := sha256.Sum256(symKey[:])
	return hex.EncodeToString(h[:])
}
