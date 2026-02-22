package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

var txCmd = &cobra.Command{
	Use:              "info",
	Short:            "Analyze and show all information about a tx",
	Long:             ``,
	TraverseChildren: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonNetworkPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)

		para := strings.Join(args, " ")
		txs := util.ScanForTxs(para)
		if len(txs) == 0 {
			appUI.Error("Couldn't find any tx hash in the params")
			return
		}

		appUI.Info("Following tx hash(es) will be analyzed shortly:")
		for i, t := range txs {
			appUI.Info("  %d. %s", i, t)
		}

		results := jarviscommon.TxResults{}

		if config.JSONOutputFile != "" {
			defer func() {
				if err := results.Write(config.JSONOutputFile); err != nil {
					appUI.Error("Writing to json file failed: %s", err)
				}
			}()
		}

		for _, t := range txs {
			appUI.Info("Analyzing tx: %s...", t)

			r := util.AnalyzeAndPrint(
				appUI,
				tc.Reader,
				tc.Analyzer,
				t,
				config.Network(),
				config.ForceERC20ABI,
				config.CustomABI,
				nil,
				nil,
				config.DegenMode,
			)
			results[t] = r
			appUI.Info("----------------------------------------------------------")
		}
	},
}

func init() {
	txCmd.PersistentFlags().BoolVarP(&config.ForceERC20ABI, "erc20-abi", "e", false, "Use ERC20 ABI where possible.")
	txCmd.PersistentFlags().StringVarP(&config.CustomABI, "abi", "c", "", "Custom abi. It can be either an address, a path to an abi file or an url to an abi. If it is an address, the abi of that address from etherscan will be queried. This param only takes effect if erc20-abi param is not true.")
	txCmd.PersistentFlags().StringVarP(&config.JSONOutputFile, "json-output", "o", "", "write output of contract read to json file")

	rootCmd.AddCommand(txCmd)
}
