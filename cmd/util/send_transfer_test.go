package util

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	jarvisutil "github.com/tranvictor/jarvis/util"
)

type stubResolver struct {
	addrs map[string]string
}

func (s stubResolver) GetAddressFromString(str string) (string, string, error) {
	return s.GetMatchingAddress(str)
}
func (s stubResolver) GetMatchingAddress(str string) (string, string, error) {
	if a, ok := s.addrs[str]; ok {
		return a, "", nil
	}
	return "", "", errors.New("not found")
}
func (s stubResolver) GetABI(string, jarvisnetworks.Network) (*abi.ABI, error) {
	return nil, errors.New("unused")
}
func (s stubResolver) ConfigToABI(string, bool, string, jarvisnetworks.Network) (*abi.ABI, error) {
	return nil, errors.New("unused")
}

func TestResolveSendTransferNative(t *testing.T) {
	if err := config.SetNetwork("mainnet"); err != nil {
		t.Fatal(err)
	}
	r := stubResolver{addrs: map[string]string{
		"alice": "0x1111111111111111111111111111111111111111",
	}}
	got, err := ResolveSendTransfer(r, "1.5", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if got.TokenAddr != jarvisutil.ETH_ADDR || got.DestAddr != "0x1111111111111111111111111111111111111111" || got.AmountStr != "1.5" {
		t.Fatalf("%+v", got)
	}
}

func TestResolveSendTransferERC20ByAddress(t *testing.T) {
	if err := config.SetNetwork("mainnet"); err != nil {
		t.Fatal(err)
	}
	token := "0x2222222222222222222222222222222222222222"
	dest := "0x3333333333333333333333333333333333333333"
	r := stubResolver{addrs: map[string]string{dest: dest}}
	got, err := ResolveSendTransfer(r, "ALL "+token, dest)
	if err != nil {
		t.Fatal(err)
	}
	if got.TokenAddr != token || got.AmountStr != "ALL" || got.DestAddr != dest {
		t.Fatalf("%+v", got)
	}
}

func TestResolveSendTransferTokenName(t *testing.T) {
	if err := config.SetNetwork("mainnet"); err != nil {
		t.Fatal(err)
	}
	r := stubResolver{addrs: map[string]string{
		"knc token": "0x4444444444444444444444444444444444444444",
		"bob":       "0x5555555555555555555555555555555555555555",
	}}
	got, err := ResolveSendTransfer(r, "0.01 knc", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if got.TokenAddr != "0x4444444444444444444444444444444444444444" || got.AmountStr != "0.01" {
		t.Fatalf("%+v", got)
	}
}

func TestResolveSendTransferNativeSymbolNotToken(t *testing.T) {
	if err := config.SetNetwork("mainnet"); err != nil {
		t.Fatal(err)
	}
	// If "ETH" were treated as a token name we would look up "ETH token"
	// and could send an ERC-20 transfer instead of native ETH.
	r := stubResolver{addrs: map[string]string{
		"ETH token": "0xdeaddeaddeaddeaddeaddeaddeaddeaddeaddead",
		"alice":     "0x1111111111111111111111111111111111111111",
	}}
	for _, value := range []string{"1 ETH", "1 eth", "ALL ETH"} {
		got, err := ResolveSendTransfer(r, value, "alice")
		if err != nil {
			t.Fatalf("%q: %v", value, err)
		}
		if got.TokenAddr != jarvisutil.ETH_ADDR {
			t.Fatalf("%q: token %s, want native ETH sentinel", value, got.TokenAddr)
		}
	}
}

func TestResolveSendTransferErrors(t *testing.T) {
	if err := config.SetNetwork("mainnet"); err != nil {
		t.Fatal(err)
	}
	r := stubResolver{addrs: map[string]string{}}
	_, err := ResolveSendTransfer(r, "1 nosuch", "0x1111111111111111111111111111111111111111")
	if !errors.Is(err, ErrSendTokenNotFound) {
		t.Fatalf("token: %v", err)
	}
	_, err = ResolveSendTransfer(r, "1", "unknown")
	if !errors.Is(err, ErrSendDestNotFound) {
		t.Fatalf("dest: %v", err)
	}
}
