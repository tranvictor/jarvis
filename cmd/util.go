package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
)

const (
	NEXT int = -1
	BACK int = -2
)

func promptIndex(prompter string, min, max int) int {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf(prompter)
	for {
		fmt.Printf("\nYou: ")
		text, _ := reader.ReadString('\n')
		indexInput := text[0 : len(text)-1]
		if indexInput == "next" {
			return NEXT
		} else if indexInput == "back" {
			return BACK
		} else {
			index, err := strconv.Atoi(indexInput)
			if err != nil {
				fmt.Printf("Jarvis: Please enter the index or 'next' or 'back'\n")
			} else if min <= index && index <= max {
				return index
			} else {
				fmt.Printf("Jarvis: Please enter the index. It should be any number from 0-4\n")
			}
		}
	}
}

func promptInput(prompter string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf(prompter)
	fmt.Printf("\nYou: ")
	text, _ := reader.ReadString('\n')
	return text[0 : len(text)-1]
}

func promptFilePath(prompter string) string {
	return promptInput(prompter)
}
