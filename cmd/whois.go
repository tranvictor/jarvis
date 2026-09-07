package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
)

var whoisCmd = &cobra.Command{
	Use:   "whois",
	Short: "Show name of one or multiple addresses",
	Run: func(cmd *cobra.Command, args []string) {
		para := strings.Join(args, " ")
		addresses := util.ScanForAddresses(para)
		if len(addresses) == 0 {
			appUI.Error("Couldn't find any addresses in the params")
			return
		}
		for _, address := range addresses {
			addrs, names, _ := util.GetExactAddressFromDatabases(address)
			if len(addrs) == 0 {
				appUI.Info("%s  %s", address, appUI.Style(ui.StyledText{Text: "not in address book", Severity: ui.SeverityMuted}))
				continue
			}
			appUI.Info("%s  %s", addrs[0], names[0])
		}
	},
}

func init() {
	rootCmd.AddCommand(whoisCmd)
}
