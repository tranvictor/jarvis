// build only if the os is not windows to use
// golang.org/x/crypto/ssh/terminal

// +build !windows

package trezoreum

import (
	"golang.org/x/crypto/ssh/terminal"
)

func ReadPassword(fd int) ([]byte, error) {
	return terminal.ReadPassword(fd)
}
