package gateways

import (
	"testing"
)

func TestVerifyOwner(t *testing.T) {
	owners := []string{"0xAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAa"}
	if err := verifyOwner("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", owners, "Safe", "0xsafe"); err != nil {
		t.Fatalf("checksum vs lower should match: %v", err)
	}
	if err := verifyOwner("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", owners, "Safe", "0xsafe"); err == nil {
		t.Fatal("expected reject")
	}
}
