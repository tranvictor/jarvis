package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/util"
)

// txCmd represents the tx command
var addressCmd = &cobra.Command{
	Use:   "addr",
	Short: "Find at max 10 matching addresses",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		para := strings.Join(args, " ")
		addrs, names, _ := util.GetMatchingAddresses(para)
		fmt.Printf("Addresses\n")
		fmt.Printf("-----------------------\n")
		for i, addr := range addrs {
			fmt.Printf("%s: %s\n", addr, names[i])
		}
	},
}

func init() {
	rootCmd.AddCommand(addressCmd)
}
