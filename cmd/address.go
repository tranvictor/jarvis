package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/db"
)

// txCmd represents the tx command
var addressCmd = &cobra.Command{
	Use:   "addr",
	Short: "Find at max 10 matching addresses",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		para := strings.Join(args, " ")
		addrs, scores := db.GetAddresses(para)
		fmt.Printf("%12s  Addresses\n", "Scores")
		fmt.Printf("-----------------------\n")
		for i, addr := range addrs {
			fmt.Printf("%12d  %s: %s\n", scores[i], addr.Address, addr.Desc)
		}
	},
}

func init() {
	rootCmd.AddCommand(addressCmd)
}
