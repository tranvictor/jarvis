package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

// txCmd represents the tx command
var txCmd = &cobra.Command{
	Use:   "info",
	Short: "Analyze and show all information about a tx",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		para := strings.Join(args, " ")
		txs := util.ScanForTxs(para)
		if len(txs) == 0 {
			fmt.Printf("Couldn't find any tx hash in the params\n")
		} else {
			fmt.Printf("Following tx hash(es) will be analyzed shortly:\n")
			for i, t := range txs {
				fmt.Printf("  %d. %s\n", i, t)
			}
			fmt.Printf("\n\n")
			analyzer, err := util.EthAnalyzer(config.Network)
			if err != nil {
				fmt.Printf("Couldn't analyze the txs: %s\n", err)
				return
			}
			for _, t := range txs {
				fmt.Printf("Analyzing tx: %s...\n", t)
				util.AnalyzeAndPrint(analyzer, t, config.Network)
				fmt.Printf("----------------------------------------------------------\n")
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(txCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// txCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// txCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
