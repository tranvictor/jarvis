package util

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

const safeProxyABIJSON = `[{"inputs":[{"internalType":"address","name":"_singleton","type":"address"}],"stateMutability":"nonpayable","type":"constructor"},{"stateMutability":"payable","type":"fallback"}]`

const uupsProxyABIJSON = `[
	{"inputs":[],"name":"implementation","outputs":[{"type":"address","name":""}],"stateMutability":"view","type":"function"},
	{"inputs":[{"name":"newImplementation","type":"address"}],"name":"upgradeTo","outputs":[],"stateMutability":"nonpayable","type":"function"},
	{"inputs":[{"name":"newImplementation","type":"address"},{"name":"data","type":"bytes"}],"name":"upgradeToAndCall","outputs":[],"stateMutability":"payable","type":"function"}
]`

func mustParseABI(t *testing.T, jsonStr string) *abi.ABI {
	t.Helper()
	parsed, err := abi.JSON(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("parse ABI: %v", err)
	}
	return &parsed
}

func TestIsMethodlessABISafeProxy(t *testing.T) {
	a := mustParseABI(t, safeProxyABIJSON)
	if !isMethodlessABI(a) {
		t.Fatal("SafeProxy ABI (constructor + fallback) must be treated as methodless")
	}
	if IsProxyABI(a) {
		t.Fatal("SafeProxy ABI does not expose upgradeTo/implementation and is not a classic proxy ABI")
	}
}

func TestIsMethodlessABINil(t *testing.T) {
	if !isMethodlessABI(nil) {
		t.Fatal("nil ABI should be methodless")
	}
}

func TestIsProxyABIUpgradeable(t *testing.T) {
	a := mustParseABI(t, uupsProxyABIJSON)
	if isMethodlessABI(a) {
		t.Fatal("UUPS proxy ABI has methods")
	}
	if !IsProxyABI(a) {
		t.Fatal("UUPS proxy ABI should match IsProxyABI")
	}
}

func TestFollowProxyImplementationSkipsNormalABI(t *testing.T) {
	erc20 := mustParseABI(t, `[
		{"inputs":[],"name":"name","outputs":[{"type":"string","name":""}],"stateMutability":"view","type":"function"},
		{"inputs":[{"name":"to","type":"address"},{"name":"value","type":"uint256"}],"name":"transfer","outputs":[{"type":"bool","name":""}],"stateMutability":"nonpayable","type":"function"}
	]`)
	impl, followed, err := followProxyImplementation("0x41675c099f32341bf84bfc5382af534df5c7461a", erc20, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if followed || impl != nil {
		t.Fatal("non-proxy ABIs must not trigger implementation lookup")
	}
}
