package common

import (
	. "github.com/logrusorgru/aurora"
)

func AlertColor(str string) string {
	return Red(str).String()
}

func InfoColor(str string) string {
	return Green(str).String()
}

func NameWithColor(name string) string {
	if name == "unknown" {
		return AlertColor(name)
	} else {
		return InfoColor(name)
	}
}
