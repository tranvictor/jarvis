package util

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tranvictor/jarvis/config"
	jarvisutil "github.com/tranvictor/jarvis/util"
)

var (
	ErrSendValueFormat   = errors.New("wrong send value format")
	ErrSendTokenNotFound = errors.New("couldn't find the token by name or address")
	ErrSendDestNotFound  = errors.New("couldn't get destination address")
)

// SendTransfer is the resolved --amount / --to pair shared by the EOA,
// Classic, and Safe send paths. AmountWei is left to the caller because
// ALL uses a different holder address in each path.
type SendTransfer struct {
	TokenAddr string
	DestAddr  string
	AmountStr string
	Currency  string
}

// ResolveSendTransfer parses value (`float`, `float TOKEN`, `ALL TOKEN`)
// and looks up the token and destination via resolver.
func ResolveSendTransfer(resolver ABIResolver, value, to string) (SendTransfer, error) {
	amountStr, currency, err := jarvisutil.ValueToAmountAndCurrency(value)
	if err != nil {
		return SendTransfer{}, fmt.Errorf("%w: %s", ErrSendValueFormat, err)
	}

	tokenAddr := jarvisutil.ETH_ADDR
	if currency != jarvisutil.ETH_ADDR && !strings.EqualFold(currency, config.Network().GetNativeTokenSymbol()) {
		addr, _, err := resolver.GetMatchingAddress(currency + " token")
		if err != nil {
			if jarvisutil.IsAddress(currency) {
				tokenAddr = currency
			} else {
				return SendTransfer{}, ErrSendTokenNotFound
			}
		} else {
			tokenAddr = addr
		}
	}

	dest, _, err := resolver.GetMatchingAddress(to)
	if err != nil {
		return SendTransfer{}, fmt.Errorf("%w: %s", ErrSendDestNotFound, to)
	}

	return SendTransfer{
		TokenAddr: tokenAddr,
		DestAddr:  dest,
		AmountStr: amountStr,
		Currency:  currency,
	}, nil
}
