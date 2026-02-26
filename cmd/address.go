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
		addrs, names, err := util.GetMatchingAddresses(para)
		if err != nil || len(addrs) == 0 {
			appUI.Warn("No matching addresses found for \"%s\"", para)
			return
		}
		appUI.Info("Found %d matching address(es):", len(addrs))
		appUI.Info("-----------------------")
		for i, addr := range addrs {
			appUI.Info("%d. %s (%s)", i+1, addr, names[i])
		}
	},
}

func init() {
	rootCmd.AddCommand(addressCmd)
}
