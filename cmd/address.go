package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/util"
)

var addressCmd = &cobra.Command{
	Use:   "addr",
	Short: "Find at max 10 matching addresses",
	Run: func(cmd *cobra.Command, args []string) {
		para := strings.Join(args, " ")
		// GetMatchingAddresses's third return is match scores, not an
		// error — it was previously misnamed `err` here, which made this
		// branch fire (and print "No matching addresses found") on every
		// successful, non-empty result too, since a populated scores
		// slice is non-nil.
		addrs, names, _ := util.GetMatchingAddresses(para)
		if len(addrs) == 0 {
			appUI.Warn("No matching addresses found for %q", para)
			return
		}
		for i, addr := range addrs {
			appUI.Info("%d. %s", i+1, jarviscommon.NameFirst(jarviscommon.Address{Address: addr, Desc: names[i]}, true))
		}
	},
}

func init() {
	rootCmd.AddCommand(addressCmd)
}
