package cmd

import (
	"fmt"
	"testing"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
)

func TestSendTransferUserErrorPreservesMessages(t *testing.T) {
	to := "alice"
	wrappedDest := fmt.Errorf("%w: %s", cmdutil.ErrSendDestNotFound, to)
	wrappedVal := fmt.Errorf("%w: bad", cmdutil.ErrSendValueFormat)

	if got := sendTransferUserError(wrappedVal, to); got != "Wrong format of the --value/-v param" {
		t.Fatalf("classic value: %q", got)
	}
	if got := sendTransferUserError(cmdutil.ErrSendTokenNotFound, to); got != "Couldn't find the token by name or address" {
		t.Fatalf("token: %q", got)
	}
	if got := sendTransferUserError(wrappedDest, to); got != "Couldn't get destination address with keyword: alice" {
		t.Fatalf("classic dest: %q", got)
	}

	if got := sendTransferUserErrorEOA(wrappedDest, to); got != "Couldn't find destination address by keyword nor address: alice" {
		t.Fatalf("eoa dest: %q", got)
	}
	if got := sendTransferUserErrorEOA(wrappedVal, to); got != "Wrong format of --value/-v param" {
		t.Fatalf("eoa value: %q", got)
	}
}
