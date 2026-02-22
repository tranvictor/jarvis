package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/util"
)

var addressCmd = &cobra.Command{
	Use:   "addr",
	Short: "Find at max 10 matching addresses",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		para := strings.Join(args, " ")
		addrs, names, _ := util.GetMatchingAddresses(para)
		appUI.Info("Addresses")
		appUI.Info("-----------------------")
		for i, addr := range addrs {
			appUI.Info("%s: %s", addr, names[i])
		}
	},
}

func init() {
	rootCmd.AddCommand(addressCmd)
}
