package util

import (
	"errors"
	"math/big"
	"testing"

	jarvisutil "github.com/tranvictor/jarvis/util"
)

type stubBalanceReader struct {
	native        *big.Int
	token         *big.Int
	decimals      uint64
	errNative     error
	errToken      error
	errDecimals   error
	lastHolder    string
	lastToken     string
	lastTokenUser string
}

func (s *stubBalanceReader) GetBalance(address string) (*big.Int, error) {
	s.lastHolder = address
	if s.errNative != nil {
		return nil, s.errNative
	}
	if s.native == nil {
		return big.NewInt(0), nil
	}
	return new(big.Int).Set(s.native), nil
}

func (s *stubBalanceReader) ERC20Balance(caddr, user string) (*big.Int, error) {
	s.lastToken = caddr
	s.lastTokenUser = user
	if s.errToken != nil {
		return nil, s.errToken
	}
	if s.token == nil {
		return big.NewInt(0), nil
	}
	return new(big.Int).Set(s.token), nil
}

func (s *stubBalanceReader) ERC20Decimal(string) (uint64, error) {
	if s.errDecimals != nil {
		return 0, s.errDecimals
	}
	if s.decimals == 0 {
		return 18, nil
	}
	return s.decimals, nil
}

const (
	testHolder = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testToken  = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestResolveSendAmountWeiNativeFixed(t *testing.T) {
	got, err := ResolveSendAmountWei(&stubBalanceReader{}, AmountWeiOpts{
		TokenAddr:      jarvisutil.ETH_ADDR,
		AmountStr:      "1.5",
		Holder:         testHolder,
		NativeDecimals: 18,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := new(big.Int).Mul(big.NewInt(15), new(big.Int).Exp(big.NewInt(10), big.NewInt(17), nil))
	if got.Cmp(want) != 0 {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestResolveSendAmountWeiNativeALLHolder(t *testing.T) {
	r := &stubBalanceReader{native: big.NewInt(1000)}
	got, err := ResolveSendAmountWei(r, AmountWeiOpts{
		TokenAddr:      jarvisutil.ETH_ADDR,
		AmountStr:      "ALL",
		Holder:         testHolder,
		NativeDecimals: 18,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.lastHolder != testHolder {
		t.Fatalf("holder %s", r.lastHolder)
	}
	if got.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("got %s", got)
	}
}

func TestResolveSendAmountWeiNativeALLSubtractsGas(t *testing.T) {
	r := &stubBalanceReader{native: big.NewInt(1000)}
	got, err := ResolveSendAmountWei(r, AmountWeiOpts{
		TokenAddr:      jarvisutil.ETH_ADDR,
		AmountStr:      "ALL",
		Holder:         testHolder,
		NativeDecimals: 18,
		SubtractGas:    big.NewInt(21),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Cmp(big.NewInt(979)) != 0 {
		t.Fatalf("got %s want 979", got)
	}
}

func TestResolveSendAmountWeiNativeALLInsufficientGas(t *testing.T) {
	r := &stubBalanceReader{native: big.NewInt(100)}
	_, err := ResolveSendAmountWei(r, AmountWeiOpts{
		TokenAddr:      jarvisutil.ETH_ADDR,
		AmountStr:      "ALL",
		Holder:         testHolder,
		NativeDecimals: 18,
		SubtractGas:    big.NewInt(21000),
	})
	if !errors.Is(err, ErrSendInsufficientGas) {
		t.Fatalf("got %v", err)
	}
}

func TestResolveSendAmountWeiERC20ALL(t *testing.T) {
	r := &stubBalanceReader{token: big.NewInt(42)}
	got, err := ResolveSendAmountWei(r, AmountWeiOpts{
		TokenAddr:      testToken,
		AmountStr:      "ALL",
		Holder:         testHolder,
		NativeDecimals: 18,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.lastToken != testToken || r.lastTokenUser != testHolder {
		t.Fatalf("token=%s user=%s", r.lastToken, r.lastTokenUser)
	}
	if got.Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("got %s", got)
	}
}

func TestResolveSendAmountWeiERC20Fixed(t *testing.T) {
	r := &stubBalanceReader{decimals: 6}
	got, err := ResolveSendAmountWei(r, AmountWeiOpts{
		TokenAddr:      testToken,
		AmountStr:      "1.5",
		Holder:         testHolder,
		NativeDecimals: 18,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Cmp(big.NewInt(1500000)) != 0 {
		t.Fatalf("got %s want 1500000", got)
	}
}

func TestResolveSendAmountWeiErrors(t *testing.T) {
	_, err := ResolveSendAmountWei(&stubBalanceReader{errNative: errors.New("rpc")}, AmountWeiOpts{
		TokenAddr: jarvisutil.ETH_ADDR, AmountStr: "ALL", Holder: testHolder, NativeDecimals: 18,
	})
	if !errors.Is(err, ErrSendNativeBalance) {
		t.Fatalf("native: %v", err)
	}

	_, err = ResolveSendAmountWei(&stubBalanceReader{errToken: errors.New("rpc")}, AmountWeiOpts{
		TokenAddr: testToken, AmountStr: "ALL", Holder: testHolder, NativeDecimals: 18,
	})
	if !errors.Is(err, ErrSendTokenBalance) {
		t.Fatalf("token bal: %v", err)
	}

	_, err = ResolveSendAmountWei(&stubBalanceReader{errDecimals: errors.New("rpc")}, AmountWeiOpts{
		TokenAddr: testToken, AmountStr: "1", Holder: testHolder, NativeDecimals: 18,
	})
	if !errors.Is(err, ErrSendTokenDecimal) {
		t.Fatalf("decimals: %v", err)
	}

	_, err = ResolveSendAmountWei(&stubBalanceReader{}, AmountWeiOpts{
		TokenAddr: jarvisutil.ETH_ADDR, AmountStr: "nope", Holder: testHolder, NativeDecimals: 18,
	})
	if !errors.Is(err, ErrSendAmountParse) {
		t.Fatalf("parse: %v", err)
	}
}

func TestAmountWeiCause(t *testing.T) {
	err := fmtWrapNative()
	if AmountWeiCause(err) != "rpc down" {
		t.Fatalf("cause = %q", AmountWeiCause(err))
	}
	if AmountWeiCause(ErrSendInsufficientGas) != ErrSendInsufficientGas.Error() {
		t.Fatalf("bare sentinel: %q", AmountWeiCause(ErrSendInsufficientGas))
	}
}

func fmtWrapNative() error {
	_, err := ResolveSendAmountWei(&stubBalanceReader{errNative: errors.New("rpc down")}, AmountWeiOpts{
		TokenAddr: jarvisutil.ETH_ADDR, AmountStr: "ALL", Holder: testHolder, NativeDecimals: 18,
	})
	return err
}
