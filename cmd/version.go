package cmd

import (
	"github.com/spf13/cobra"
)

var VERSION string = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show jarvis version",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		appUI.Info("Version: %s", VERSION)
		appUI.Info("Contact author at: @tranvictor on Telegram or victor@kyber.network")
		appUI.Info("At Kyber, our objective is to grow as a respected team in crypto world")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
