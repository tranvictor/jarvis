package wccrypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// WalletConnect relay-auth JWT (https://docs.walletconnect.com/cloud/relay)
//
// The WC relay requires every subscriber to present a JWT signed with
// a client-owned ed25519 key. The public key becomes the issuer, and
// the relay URL becomes the audience. There is no registration step —
// the relay trusts any well-formed JWT from any ed25519 key, on the
// assumption that rate-limits are scoped to the projectId instead.
//
// The JWT has the usual three parts (header.payload.signature), all
// base64url-unpadded. The issuer is a did:key identifier encoding the
// ed25519 public key via multibase base58btc + multicodec 0xed01.

// RelayKeyPair is an ed25519 identity used to sign the relay-auth JWT.
// Regenerated per process start; callers discard it when the session
// ends.
type RelayKeyPair struct {
	Public  ed25519.PublicKey
	Private ed25519.PrivateKey
}

// NewRelayKeyPair generates a fresh ed25519 keypair.
func NewRelayKeyPair() (*RelayKeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ed25519 GenerateKey: %w", err)
	}
	return &RelayKeyPair{Public: pub, Private: priv}, nil
}

// DIDKey returns the did:key URI for the public key component. Used
// both as the "iss" claim in the JWT and for debug logging.
func (k *RelayKeyPair) DIDKey() string {
	// ed25519-pub multicodec prefix is 0xed, which encodes as the
	// two-byte varint 0xed 0x01.
	prefixed := make([]byte, 0, 2+ed25519.PublicKeySize)
	prefixed = append(prefixed, 0xed, 0x01)
	prefixed = append(prefixed, k.Public...)
	return "did:key:z" + Base58BTCEncode(prefixed)
}

// BuildRelayJWT produces a signed JWT suitable for the WC relay's
// ?auth= query parameter. aud is the relay URL (typically
// "wss://relay.walletconnect.org"); ttl is how long the JWT remains
// valid (the relay enforces exp).
func BuildRelayJWT(k *RelayKeyPair, aud string, ttl time.Duration) (string, error) {
	header := map[string]string{
		"alg": "EdDSA",
		"typ": "JWT",
	}
	// sub is meant to be a per-connection random 32-byte hex string.
	var subBytes [32]byte
	if _, err := io.ReadFull(rand.Reader, subBytes[:]); err != nil {
		return "", fmt.Errorf("read sub bytes: %w", err)
	}
	now := time.Now().Unix()
	payload := map[string]interface{}{
		"iat": now,
		"exp": now + int64(ttl.Seconds()),
		"iss": k.DIDKey(),
		"sub": hex.EncodeToString(subBytes[:]),
		"aud": aud,
	}

	hEnc, err := b64JSON(header)
	if err != nil {
		return "", fmt.Errorf("encode header: %w", err)
	}
	pEnc, err := b64JSON(payload)
	if err != nil {
		return "", fmt.Errorf("encode payload: %w", err)
	}

	signingInput := hEnc + "." + pEnc
	sig := ed25519.Sign(k.Private, []byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// b64JSON marshals v to JSON and returns the unpadded-base64url form.
func b64JSON(v interface{}) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
