package cmd

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/spf13/cobra"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/reader"
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
		nwks, txs := cmdutil.ScanForTxs(para)
		if len(txs) == 0 {
			appUI.Error("Couldn't find any tx hash in the params")
			return
		}

		if len(txs) > 1 {
			appUI.Info("Analyzing %d transactions:", len(txs))
			for i, t := range txs {
				appUI.Info("  %d. %s:%s", i+1, nwks[i], t)
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

		envs := map[string]infoTxEnv{}
		for i, t := range txs {
			if i > 0 {
				appUI.Info("")
			}
			if len(txs) > 1 {
				appUI.Info("%s:%s", nwks[i], t)
			}
			env, err := infoTxEnvFor(tc, envs, nwks[i])
			if err != nil {
				appUI.Error("%s network is not supported. Skip.", nwks[i])
				continue
			}
			d := util.AnalyzeAndPrint(
				appUI,
				env.reader,
				env.analyzer,
				t,
				env.network,
				config.ForceERC20ABI,
				config.CustomABI,
				nil,
				nil,
				util.InfoLayout(config.DegenMode),
				cmdutil.InfoClearSign(env.network),
			)
			displays[t] = d
		}
	},
}

type infoTxEnv struct {
	network  jarvisnetworks.Network
	reader   reader.Reader
	analyzer util.TxAnalyzer
}

func infoTxEnvFor(tc cmdutil.TxContext, cache map[string]infoTxEnv, name string) (infoTxEnv, error) {
	if env, ok := cache[name]; ok {
		return env, nil
	}
	n, err := jarvisnetworks.GetNetwork(name)
	if err != nil {
		return infoTxEnv{}, err
	}
	key := n.GetName()
	if env, ok := cache[key]; ok {
		cache[name] = env
		return env, nil
	}
	env := infoTxEnv{network: n}
	if tc.Reader != nil && key == config.Network().GetName() {
		env.reader = tc.Reader
		env.analyzer = tc.Analyzer
	} else {
		r, err := util.EthReader(n)
		if err != nil {
			return infoTxEnv{}, err
		}
		env.reader = r
		env.analyzer = txanalyzer.NewGenericAnalyzer(r, n)
	}
	cache[name] = env
	cache[key] = env
	return env, nil
}

func init() {
	txCmd.PersistentFlags().BoolVarP(&config.ForceERC20ABI, "erc20-abi", "e", false, "Use ERC20 ABI where possible.")
	txCmd.PersistentFlags().StringVarP(&config.CustomABI, "abi", "c", "", customABIFlagHelp)
	txCmd.PersistentFlags().StringVarP(&config.JSONOutputFile, "json-output", "o", "", "write the analysed transaction display to a JSON file")

	rootCmd.AddCommand(txCmd)
}
