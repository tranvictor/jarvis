package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	. "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/txanalyzer"
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

			reader, err := util.EthReader(config.Network())
			if err != nil {
				fmt.Printf("Couldn't init eth reader: %s\n", err)
				return
			}

			analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

			results := TxResults{}

			if config.JSONOutputFile != "" {
				defer results.Write(config.JSONOutputFile)
			}

			for _, t := range txs {
				fmt.Printf("Analyzing tx: %s...\n", t)

				r := util.AnalyzeAndPrint(
					reader,
					analyzer,
					t,
					config.Network(),
					config.ForceERC20ABI,
					config.CustomABI,
					nil,
					nil,
					config.DegenMode,
				)
				results[t] = r
				fmt.Printf("----------------------------------------------------------\n")
			}
		}
	},
}

func init() {
	txCmd.PersistentFlags().BoolVarP(&config.ForceERC20ABI, "erc20-abi", "e", false, "Use ERC20 ABI where possible.")
	txCmd.PersistentFlags().StringVarP(&config.CustomABI, "abi", "c", "", "Custom abi. It can be either an address, a path to an abi file or an url to an abi. If it is an address, the abi of that address from etherscan will be queried. This param only takes effect if erc20-abi param is not true.")
	txCmd.PersistentFlags().StringVarP(&config.JSONOutputFile, "json-output", "o", "", "write output of contract read to json file")

	rootCmd.AddCommand(txCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// txCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// txCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
