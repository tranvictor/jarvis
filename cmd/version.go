package cmd

import (
	"github.com/spf13/cobra"
)

var VERSION string = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show jarvis version",
	Run: func(cmd *cobra.Command, args []string) {
		appUI.Info("jarvis %s", VERSION)
		appUI.Info("https://github.com/tranvictor/jarvis")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
