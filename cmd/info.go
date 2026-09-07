package cmd

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/spf13/cobra"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
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
		txs := util.ScanForTxHashes(para)
		if len(txs) == 0 {
			appUI.Error("Couldn't find any tx hash in the params")
			return
		}

		if len(txs) > 1 {
			appUI.Info("Analyzing %d transactions:", len(txs))
			for i, t := range txs {
				appUI.Info("  %d. %s", i+1, t)
			}
		}

		displays := map[string]*util.TxDisplay{}

		if config.JSONOutputFile != "" {
			defer func() {
				data, _ := json.MarshalIndent(displays, "", "  ")
				if err := os.WriteFile(config.JSONOutputFile, data, 0644); err != nil {
					appUI.Error("Writing to json file failed: %s", err)
				}
			}()
		}

		for i, t := range txs {
			if i > 0 {
				appUI.Info("")
			}
			if len(txs) > 1 {
				appUI.Info("%s", t)
			}
			d := util.AnalyzeAndPrint(
				appUI,
				tc.Reader,
				tc.Analyzer,
				t,
				config.Network(),
				config.ForceERC20ABI,
				config.CustomABI,
				nil,
				nil,
				util.InfoLayout(config.DegenMode),
				cmdutil.InfoClearSign(config.Network()),
			)
			displays[t] = d
		}
	},
}

func init() {
	txCmd.PersistentFlags().BoolVarP(&config.ForceERC20ABI, "erc20-abi", "e", false, "Use ERC20 ABI where possible.")
	txCmd.PersistentFlags().StringVarP(&config.CustomABI, "abi", "c", "", customABIFlagHelp)
	txCmd.PersistentFlags().StringVarP(&config.JSONOutputFile, "json-output", "o", "", "write the analysed transaction display to a JSON file")

	rootCmd.AddCommand(txCmd)
}
