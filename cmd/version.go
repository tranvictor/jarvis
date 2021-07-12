package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	VERSION string = "0.0.21"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show jarvis version",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Version: %s\n", VERSION)
		fmt.Printf("Contact author at: @tranvictor on Telegram or victor@kyber.network\n")
		fmt.Printf("At Kyber, besides contributing liquidity, we also contribute tools :)")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
