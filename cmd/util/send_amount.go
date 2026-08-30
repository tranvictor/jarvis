package util

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	jarviscommon "github.com/tranvictor/jarvis/common"
	jarvisutil "github.com/tranvictor/jarvis/util"
)

var (
	ErrSendNativeBalance   = errors.New("couldn't get native balance")
	ErrSendTokenBalance    = errors.New("couldn't get token balance")
	ErrSendTokenDecimal    = errors.New("couldn't get token decimal")
	ErrSendAmountParse     = errors.New("couldn't calculate amount")
	ErrSendInsufficientGas = errors.New("insufficient balance to cover gas")
)

// BalanceReader is the subset of util/reader.Reader used to turn an
// amount string into wei.
type BalanceReader interface {
	GetBalance(address string) (*big.Int, error)
	ERC20Balance(caddr string, user string) (*big.Int, error)
	ERC20Decimal(caddr string) (uint64, error)
}

// AmountWeiOpts controls ResolveSendAmountWei.
// Holder is the address ALL reads (EOA, Classic msig, or Safe).
// SubtractGas, when non-nil, is subtracted from a native ALL balance
// (EOA send); Classic/Safe leave it nil so ALL is the full holder balance.
type AmountWeiOpts struct {
	TokenAddr      string
	AmountStr      string
	Holder         string
	NativeDecimals uint64
	SubtractGas    *big.Int
}

// ResolveSendAmountWei converts AmountStr (`ALL` or a decimal float) to wei.
func ResolveSendAmountWei(r BalanceReader, opts AmountWeiOpts) (*big.Int, error) {
	if opts.TokenAddr == jarvisutil.ETH_ADDR {
		if opts.AmountStr == "ALL" {
			bal, err := r.GetBalance(opts.Holder)
			if err != nil {
				return nil, fmt.Errorf("%w: %w", ErrSendNativeBalance, err)
			}
			if opts.SubtractGas != nil {
				if bal.Cmp(opts.SubtractGas) < 0 {
					return nil, ErrSendInsufficientGas
				}
				return new(big.Int).Sub(bal, opts.SubtractGas), nil
			}
			return bal, nil
		}
		amount, err := jarviscommon.FloatStringToBig(opts.AmountStr, opts.NativeDecimals)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSendAmountParse, err)
		}
		return amount, nil
	}

	if opts.AmountStr == "ALL" {
		bal, err := r.ERC20Balance(opts.TokenAddr, opts.Holder)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSendTokenBalance, err)
		}
		return bal, nil
	}
	decimals, err := r.ERC20Decimal(opts.TokenAddr)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSendTokenDecimal, err)
	}
	amount, err := jarviscommon.FloatStringToBig(opts.AmountStr, decimals)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSendAmountParse, err)
	}
	return amount, nil
}

// AmountWeiCause returns the wrapped reader/parse error text so callers
// can keep their original "Couldn't …: %s" strings.
func AmountWeiCause(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	for _, s := range []error{
		ErrSendNativeBalance,
		ErrSendTokenBalance,
		ErrSendTokenDecimal,
		ErrSendAmountParse,
	} {
		prefix := s.Error() + ": "
		if strings.HasPrefix(msg, prefix) {
			return strings.TrimPrefix(msg, prefix)
		}
	}
	return msg
}
