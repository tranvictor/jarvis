package trezoreum

import (
	"fmt"
	"syscall"
)

func getPassword(prompt string) string {
	fmt.Print(prompt)
	bytePassword, _ := ReadPassword(int(syscall.Stdin))
	fmt.Printf("\n")
	return string(bytePassword)
}

func PromptPINFromStdin() string {
	return getPassword(
		"Pin required to open Trezor wallet\n" +
			"Look at the device for number positions\n\n" +
			"7 | 8 | 9\n" +
			"--+---+--\n" +
			"4 | 5 | 6\n" +
			"--+---+--\n" +
			"1 | 2 | 3\n\n" +
			"Enter your PIN: ",
	)
}
