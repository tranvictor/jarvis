package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/util"
)

// txCmd represents the tx command
var whoisCmd = &cobra.Command{
	Use:   "whois",
	Short: "Show name of one or multiple addresses",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		para := strings.Join(args, " ")
		addresses := util.ScanForAddresses(para)
		if len(addresses) == 0 {
			fmt.Printf("Couldn't find any addresses in the params\n")
		} else {
			for _, address := range addresses {
				addr, name, err := util.GetMatchingAddress(address)
				if err != nil {
					fmt.Printf("%s: %s\n", addr, "not found")
					continue
				}
				fmt.Printf("%s: %s\n", addr, name)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(whoisCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// txCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// txCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
