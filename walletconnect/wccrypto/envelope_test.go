package wccrypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	var k [32]byte
	for i := range k {
		k[i] = byte(i)
	}
	msg := []byte(`{"hello":"world"}`)
	env, err := Encrypt(k, msg)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	out, err := Decrypt(k, env)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if out.Type != 0 {
		t.Errorf("type = %d, want 0", out.Type)
	}
	if !bytes.Equal(out.Plaintext, msg) {
		t.Errorf("plaintext mismatch: got %q want %q", out.Plaintext, msg)
	}
}

func TestEncryptWithSenderRoundTrip(t *testing.T) {
	var k, pub [32]byte
	for i := range k {
		k[i] = byte(0x10 + i)
		pub[i] = byte(0xa0 + i)
	}
	msg := []byte(`{"result":42}`)
	env, err := EncryptWithSender(k, pub, msg)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	out, err := Decrypt(k, env)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if out.Type != 1 {
		t.Errorf("type = %d, want 1", out.Type)
	}
	if out.SenderPub != pub {
		t.Errorf("sender pub mismatch: got %x want %x", out.SenderPub, pub)
	}
	if !bytes.Equal(out.Plaintext, msg) {
		t.Errorf("plaintext mismatch")
	}
}
