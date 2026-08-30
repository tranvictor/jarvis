package cmd

import (
	"errors"
	"fmt"
	"testing"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

func TestSendAmountUserErrorPreservesMessages(t *testing.T) {
	rpc := errors.New("rpc failed")
	nativeBal := fmt.Errorf("%w: %w", cmdutil.ErrSendNativeBalance, rpc)
	tokenBal := fmt.Errorf("%w: %w", cmdutil.ErrSendTokenBalance, rpc)
	decimals := fmt.Errorf("%w: %w", cmdutil.ErrSendTokenDecimal, rpc)
	parse := fmt.Errorf("%w: %w", cmdutil.ErrSendAmountParse, rpc)

	if got := sendAmountUserErrorClassic(nativeBal, util.ETH_ADDR); got != "Couldn't get balance of the multisig: rpc failed" {
		t.Fatalf("classic native ALL: %q", got)
	}
	if got := sendAmountUserErrorClassic(parse, util.ETH_ADDR); got != "Couldn't calculate the amount: rpc failed" {
		t.Fatalf("classic native parse: %q", got)
	}
	if got := sendAmountUserErrorClassic(tokenBal, "0x1"); got != "Couldn't read balance of the multisig: rpc failed" {
		t.Fatalf("classic token ALL: %q", got)
	}
	if got := sendAmountUserErrorClassic(decimals, "0x1"); got != "Couldn't get token decimal: rpc failed" {
		t.Fatalf("classic decimals: %q", got)
	}
	if got := sendAmountUserErrorClassic(parse, "0x1"); got != "Couldn't calculate amount in wei: rpc failed" {
		t.Fatalf("classic token parse: %q", got)
	}

	if got := sendAmountUserErrorSafe(nativeBal, util.ETH_ADDR); got != "Couldn't get balance of the safe: rpc failed" {
		t.Fatalf("safe native ALL: %q", got)
	}
	if got := sendAmountUserErrorSafe(tokenBal, "0x1"); got != "Couldn't read token balance of the safe: rpc failed" {
		t.Fatalf("safe token ALL: %q", got)
	}
	if got := sendAmountUserErrorSafe(parse, util.ETH_ADDR); got != "Couldn't calculate the amount: rpc failed" {
		t.Fatalf("safe native parse: %q", got)
	}
	if got := sendAmountUserErrorSafe(parse, "0x1"); got != "Couldn't calculate amount in wei: rpc failed" {
		t.Fatalf("safe token parse: %q", got)
	}

	if err := config.SetNetwork("mainnet"); err != nil {
		t.Fatal(err)
	}
	if got := sendAmountUserErrorEOA(cmdutil.ErrSendInsufficientGas, util.ETH_ADDR); got != "Wallet doesn't have enough token to cover gas. Aborted." {
		t.Fatalf("eoa gas: %q", got)
	}
	if got := sendAmountUserErrorEOA(nativeBal, util.ETH_ADDR); got != "Couldn't get ETH balance: rpc failed" {
		t.Fatalf("eoa native ALL: %q", got)
	}
	if got := sendAmountUserErrorEOA(parse, util.ETH_ADDR); got != "Couldn't calculate send amount: rpc failed" {
		t.Fatalf("eoa native parse: %q", got)
	}
	if got := sendAmountUserErrorEOA(tokenBal, "0x1"); got != "Couldn't get token balance: rpc failed" {
		t.Fatalf("eoa token ALL: %q", got)
	}
	if got := sendAmountUserErrorEOA(parse, "0x1"); got != "Couldn't calculate token amount in wei: rpc failed" {
		t.Fatalf("eoa token parse: %q", got)
	}
}
