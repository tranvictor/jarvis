package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	VERSION string = "0.0.31"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show jarvis version",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Version: %s\n", VERSION)
		fmt.Printf("Contact author at: @tranvictor on Telegram or victor@kyber.network\n")
		fmt.Printf("At Kyber, our objective is to grow as a respected team in crypto world\n")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
