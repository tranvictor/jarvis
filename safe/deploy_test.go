package safe

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestPreferL2Singleton(t *testing.T) {
	l1 := []uint64{1, 11155111, 17000, 560048}
	for _, id := range l1 {
		if preferL2Singleton(id) {
			t.Errorf("chain %d should use the L1 Safe singleton", id)
		}
	}
	for _, id := range []uint64{10, 56, 137, 8453, 42161} {
		if !preferL2Singleton(id) {
			t.Errorf("chain %d should use SafeL2", id)
		}
	}
}

func TestValidateNewSafeParams(t *testing.T) {
	a := common.HexToAddress("0x1111111111111111111111111111111111111111")
	b := common.HexToAddress("0x2222222222222222222222222222222222222222")

	cases := []struct {
		name      string
		owners    []common.Address
		threshold *big.Int
		wantErr   string
	}{
		{"ok 1-of-1", []common.Address{a}, big.NewInt(1), ""},
		{"ok 2-of-2", []common.Address{a, b}, big.NewInt(2), ""},
		{"no owners", nil, big.NewInt(1), "at least one owner"},
		{"threshold 0", []common.Address{a}, big.NewInt(0), "at least 1"},
		{"threshold nil", []common.Address{a}, nil, "at least 1"},
		{"threshold too high", []common.Address{a}, big.NewInt(2), "greater than owner count"},
		{"zero owner", []common.Address{{}}, big.NewInt(1), "zero address"},
		{"duplicate", []common.Address{a, a}, big.NewInt(1), "duplicate owner"},
	}
	for _, tc := range cases {
		err := ValidateNewSafeParams(tc.owners, tc.threshold)
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%s: unexpected error: %s", tc.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: got %v, want substring %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestEncodeSetupSelectorAndRoundTrip(t *testing.T) {
	owners := []common.Address{
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
	}
	fallback := common.HexToAddress("0xfd0732Dc9E303f09fCEf3a7388Ad10A83459Ec99")
	got, err := EncodeSetup(owners, big.NewInt(2), fallback)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got[:4]) != "b63e800d" {
		t.Fatalf("setup selector = %s, want b63e800d", hex.EncodeToString(got[:4]))
	}

	unpacked, err := GetSafeSetupABI().Methods["setup"].Inputs.Unpack(got[4:])
	if err != nil {
		t.Fatal(err)
	}
	gotOwners := unpacked[0].([]common.Address)
	if len(gotOwners) != 2 || gotOwners[0] != owners[0] || gotOwners[1] != owners[1] {
		t.Errorf("owners = %v", gotOwners)
	}
	if unpacked[1].(*big.Int).Cmp(big.NewInt(2)) != 0 {
		t.Errorf("threshold = %v", unpacked[1])
	}
	if unpacked[4].(common.Address) != fallback {
		t.Errorf("fallback = %s", unpacked[4].(common.Address).Hex())
	}
	if unpacked[2].(common.Address) != (common.Address{}) {
		t.Error("to should be zero")
	}
	if len(unpacked[3].([]byte)) != 0 {
		t.Error("data should be empty")
	}
}

func TestEncodeSetupRejectsBadParams(t *testing.T) {
	if _, err := EncodeSetup(nil, big.NewInt(1), common.Address{}); err == nil {
		t.Fatal("expected error for empty owners")
	}
}

func TestEncodeCreateProxyWithNonceSelector(t *testing.T) {
	singleton := common.HexToAddress("0x41675C099F32341bf84BFc5382aF534df5C7461a")
	got, err := EncodeCreateProxyWithNonce(singleton, []byte{0xab}, big.NewInt(7))
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got[:4]) != "1688f0b9" {
		t.Fatalf("createProxyWithNonce selector = %s, want 1688f0b9", hex.EncodeToString(got[:4]))
	}

	unpacked, err := GetProxyFactoryABI().Methods["createProxyWithNonce"].Inputs.Unpack(got[4:])
	if err != nil {
		t.Fatal(err)
	}
	if unpacked[0].(common.Address) != singleton {
		t.Errorf("singleton = %s", unpacked[0].(common.Address).Hex())
	}
	if string(unpacked[1].([]byte)) != string([]byte{0xab}) {
		t.Errorf("initializer = %x", unpacked[1].([]byte))
	}
	if unpacked[2].(*big.Int).Cmp(big.NewInt(7)) != 0 {
		t.Errorf("salt = %v", unpacked[2])
	}

	if _, err := EncodeCreateProxyWithNonce(common.Address{}, nil, big.NewInt(0)); err == nil {
		t.Error("expected error for zero singleton")
	}
	if _, err := EncodeCreateProxyWithNonce(singleton, nil, big.NewInt(-1)); err == nil {
		t.Error("expected error for negative salt")
	}
}

func TestPredictProxyAddressMatchesCreate2Formula(t *testing.T) {
	factory := common.HexToAddress("0x4e1DCf7AD4e460CfD30791CCC4F9c8a4f820ec67")
	singleton := common.HexToAddress("0x41675C099F32341bf84BFc5382aF534df5C7461a")
	// Minimal stand-in for proxy creation code: only the hash matters.
	creation := []byte{0x60, 0x80, 0x60, 0x40}
	initializer := []byte{0xb6, 0x3e, 0x80, 0x0d}
	saltNonce := big.NewInt(42)

	got, err := PredictProxyAddress(factory, singleton, creation, initializer, saltNonce)
	if err != nil {
		t.Fatal(err)
	}

	initHash := crypto.Keccak256(initializer)
	salt := crypto.Keccak256Hash(append(initHash, common.LeftPadBytes(saltNonce.Bytes(), 32)...))
	deployment := append(append([]byte{}, creation...), common.LeftPadBytes(singleton.Bytes(), 32)...)
	want := crypto.CreateAddress2(factory, salt, crypto.Keccak256(deployment))
	if got != want {
		t.Fatalf("predicted %s, want %s", got.Hex(), want.Hex())
	}
	// Pin the hex so a formula regression fails even if both sides drift together.
	if got.Hex() != "0x83a395F3485eaff1E5491730aCDFFdD69c0a5886" {
		t.Fatalf("CREATE2 address drifted: %s", got.Hex())
	}

	if _, err := PredictProxyAddress(common.Address{}, singleton, creation, initializer, saltNonce); err == nil {
		t.Error("expected error for zero factory")
	}
	if _, err := PredictProxyAddress(factory, singleton, nil, initializer, saltNonce); err == nil {
		t.Error("expected error for empty creation code")
	}
}

func TestPredictProxyAddressNilSaltIsZero(t *testing.T) {
	factory := common.HexToAddress("0x4e1DCf7AD4e460CfD30791CCC4F9c8a4f820ec67")
	singleton := common.HexToAddress("0x29fcB43b46531BcA003ddC8FCB67FFE9194148C4")
	creation := []byte{0x01}
	withZero, err := PredictProxyAddress(factory, singleton, creation, nil, big.NewInt(0))
	if err != nil {
		t.Fatal(err)
	}
	withNil, err := PredictProxyAddress(factory, singleton, creation, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if withZero != withNil {
		t.Fatalf("nil salt %s != zero salt %s", withNil.Hex(), withZero.Hex())
	}
}

func TestResolveSafeDeploymentAllOverrides(t *testing.T) {
	factory := "0x0000000000000000000000000000000000000001"
	singleton := "0x0000000000000000000000000000000000000002"
	fallback := "0x0000000000000000000000000000000000000003"
	d, err := resolveSafeDeployment(1, func(string) (bool, error) {
		t.Fatal("should not probe when every address is overridden")
		return false, nil
	}, DeployOverrides{Factory: factory, Singleton: singleton, FallbackHandler: fallback})
	if err != nil {
		t.Fatal(err)
	}
	if d.Factory.Hex() != common.HexToAddress(factory).Hex() {
		t.Errorf("factory = %s", d.Factory.Hex())
	}
	if d.Singleton.Hex() != common.HexToAddress(singleton).Hex() {
		t.Errorf("singleton = %s", d.Singleton.Hex())
	}
	if d.FallbackHandler.Hex() != common.HexToAddress(fallback).Hex() {
		t.Errorf("fallback = %s", d.FallbackHandler.Hex())
	}
	if d.Version != "custom" {
		t.Errorf("version = %s", d.Version)
	}
}

func TestResolveSafeDeploymentBadOverride(t *testing.T) {
	_, err := resolveSafeDeployment(1, func(string) (bool, error) { return true, nil }, DeployOverrides{Factory: "nope"})
	if err == nil || !strings.Contains(err.Error(), "--factory") {
		t.Fatalf("got %v", err)
	}
}

func TestResolveSafeDeploymentProbes141ThenL1OnEthereum(t *testing.T) {
	wantFactory := strings.ToLower(safeRelease141.Factories[0].Address)
	wantSafe := strings.ToLower(safeRelease141.Safe[0].Address)
	wantFallback := strings.ToLower(safeRelease141.Fallback[0].Address)

	d, err := resolveSafeDeployment(1, func(addr string) (bool, error) {
		switch strings.ToLower(addr) {
		case wantFactory, wantSafe, wantFallback:
			return true, nil
		default:
			return false, nil
		}
	}, DeployOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Version != "1.4.1" {
		t.Errorf("version = %s", d.Version)
	}
	if !strings.EqualFold(d.Factory.Hex(), wantFactory) {
		t.Errorf("factory = %s", d.Factory.Hex())
	}
	if !strings.EqualFold(d.Singleton.Hex(), wantSafe) {
		t.Errorf("singleton = %s, want L1 Safe", d.Singleton.Hex())
	}
	if !strings.Contains(d.SingletonLabel, "Safe 1.4.1") {
		t.Errorf("singleton label = %s", d.SingletonLabel)
	}
}

func TestResolveSafeDeploymentPrefersL2OnBSC(t *testing.T) {
	wantL2 := strings.ToLower(safeRelease141.SafeL2[0].Address)
	d, err := resolveSafeDeployment(56, func(addr string) (bool, error) {
		return true, nil
	}, DeployOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(d.Singleton.Hex(), wantL2) {
		t.Errorf("singleton = %s, want SafeL2 %s", d.Singleton.Hex(), wantL2)
	}
}

func TestResolveSafeDeploymentFallsBackTo130(t *testing.T) {
	wantFactory := strings.ToLower(safeRelease130.Factories[0].Address)
	wantL2 := strings.ToLower(safeRelease130.SafeL2[0].Address)
	wantFallback := strings.ToLower(safeRelease130.Fallback[0].Address)

	d, err := resolveSafeDeployment(137, func(addr string) (bool, error) {
		switch strings.ToLower(addr) {
		case wantFactory, wantL2, wantFallback:
			return true, nil
		default:
			return false, nil
		}
	}, DeployOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Version != "1.3.0" {
		t.Errorf("version = %s", d.Version)
	}
	if !strings.EqualFold(d.Singleton.Hex(), wantL2) {
		t.Errorf("singleton = %s", d.Singleton.Hex())
	}
}

func TestResolveSafeDeploymentPartialOverride(t *testing.T) {
	customFactory := "0x00000000000000000000000000000000000000fA"
	d, err := resolveSafeDeployment(1, func(addr string) (bool, error) {
		return !strings.EqualFold(addr, customFactory), nil
	}, DeployOverrides{Factory: customFactory})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(d.Factory.Hex(), customFactory) {
		t.Errorf("factory = %s", d.Factory.Hex())
	}
	if !strings.Contains(d.FactoryLabel, "--factory") {
		t.Errorf("factory label = %s", d.FactoryLabel)
	}
	if d.Singleton == (common.Address{}) || d.FallbackHandler == (common.Address{}) {
		t.Error("expected probed singleton and fallback")
	}
}

func TestResolveSafeDeploymentNothingOnChain(t *testing.T) {
	_, err := resolveSafeDeployment(999, func(string) (bool, error) { return false, nil }, DeployOverrides{})
	if err == nil || !strings.Contains(err.Error(), "--factory") {
		t.Fatalf("got %v", err)
	}
}

func TestReleasesNewestFirstDoesNotAlias(t *testing.T) {
	before141 := len(safeRelease141.Factories)
	before130 := len(safeRelease130.Factories)
	_ = releasesNewestFirst()
	if len(safeRelease141.Factories) != before141 || len(safeRelease130.Factories) != before130 {
		t.Fatal("release tables were mutated")
	}
}
